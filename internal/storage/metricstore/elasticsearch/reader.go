// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/olivere/elastic/v7"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

var ErrNotImplemented = errors.New("metrics querying is currently not implemented yet")

const minStep = time.Millisecond

// MetricsReader orchestrates metrics queries by:
// 1. Calculating time ranges from query parameters.
// 2. Delegating query construction and execution to Query.
// 3. Using Translator to convert raw results to the domain model.
// 4. Applying metric-specific processing to get desired metrics.
type MetricsReader struct {
	queryLogger  *QueryLogger
	queryBuilder *QueryBuilder
}

// TimeRange represents a time range for metrics queries.
type TimeRange struct {
	startTimeMillis int64
	endTimeMillis   int64
	// extendedStartTimeMillis is an extended start time used for lookback periods
	// in certain aggregations (e.g., cumulative sums or rate calculations)
	// where data prior to startTimeMillis is needed to compute metrics accurately
	// within the primary time range. This typically accounts for a window of
	// preceding data (e.g., 10 minutes) to ensure that the initial data
	// points in the primary time range have enough historical context for calculation.
	extendedStartTimeMillis int64
}

// MetricsQueryParams contains parameters for Elasticsearch metrics queries.
type MetricsQueryParams struct {
	metricstore.BaseQueryParameters
	metricName string
	metricDesc string
	boolQuery  elastic.BoolQuery
	aggQuery   elastic.Aggregation
}

// Pair represents a timestamp-value pair for metrics.
type Pair struct {
	TimeStamp int64
	Value     float64
}

// NewMetricsReader initializes a new MetricsReader.
func NewMetricsReader(client es.Client, cfg config.Configuration, logger *zap.Logger, tracer trace.TracerProvider) *MetricsReader {
	tr := tracer.Tracer("elasticsearch-metricstore")
	return &MetricsReader{
		queryLogger:  NewQueryLogger(logger, tr),
		queryBuilder: NewQueryBuilder(client, cfg),
	}
}

// GetLatencies retrieves latency metrics
func (r MetricsReader) GetLatencies(ctx context.Context, params *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	timeRange, err := calculateTimeRange(&params.BaseQueryParameters)
	if err != nil {
		return nil, err
	}

	metricsParams := MetricsQueryParams{
		BaseQueryParameters: params.BaseQueryParameters,
		metricName:          "service_latencies",
		metricDesc:          fmt.Sprintf("%.2fth quantile latency, grouped by service", params.Quantile),
		boolQuery:           r.queryBuilder.BuildBoolQuery(params.BaseQueryParameters, timeRange),
		aggQuery:            r.queryBuilder.BuildLatenciesAggQuery(params, timeRange),
	}

	searchResult, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}

	translator := NewTranslator(func(
		buckets []*elastic.AggregationBucketHistogramItem,
	) []*Pair {
		return bucketsToLatencies(buckets, params.Quantile*100)
	})
	rawMetricFamily, err := translator.ToDomainMetricsFamily(metricsParams, searchResult)
	if err != nil {
		return nil, err
	}

	// Process the raw aggregation value to calculate latencies (ms)
	const lookback = 1 // only current value
	return applySlidingWindow(rawMetricFamily, lookback, scaleToMillisAndRound), nil
}

// GetCallRates retrieves call rate metrics
func (r MetricsReader) GetCallRates(ctx context.Context, params *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	timeRange, err := calculateTimeRange(&params.BaseQueryParameters)
	if err != nil {
		return nil, err
	}

	metricsParams := MetricsQueryParams{
		BaseQueryParameters: params.BaseQueryParameters,
		metricName:          "service_call_rate",
		metricDesc:          "calls/sec, grouped by service",
		boolQuery:           r.queryBuilder.BuildBoolQuery(params.BaseQueryParameters, timeRange),
		aggQuery:            r.queryBuilder.BuildCallRateAggQuery(params.BaseQueryParameters, timeRange),
	}

	searchResult, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}
	// Convert search results into raw metric family using translator
	translator := NewTranslator(bucketsToCallRate)
	rawMetricFamily, err := translator.ToDomainMetricsFamily(metricsParams, searchResult)
	if err != nil {
		return nil, err
	}

	// Process and return results
	processedMetricFamily := calcCallRate(rawMetricFamily, params.BaseQueryParameters)
	// Trim results to original time range
	return trimMetricPointsBefore(processedMetricFamily, timeRange.startTimeMillis), nil
}

// GetErrorRates retrieves error rate metrics
func (MetricsReader) GetErrorRates(_ context.Context, _ *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

// GetMinStepDuration returns the minimum step duration.
func (MetricsReader) GetMinStepDuration(_ context.Context, _ *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
}

// trimMetricPointsBefore removes metric points older than startMillis from each metric in the MetricFamily.
func trimMetricPointsBefore(mf *metrics.MetricFamily, startMillis int64) *metrics.MetricFamily {
	for _, metric := range mf.Metrics {
		points := metric.MetricPoints
		// Find first index where point >= startMillis
		cutoff := 0
		for ; cutoff < len(points); cutoff++ {
			point := points[cutoff]
			pointMillis := point.Timestamp.Seconds*1000 + int64(point.Timestamp.Nanos)/1000000
			if pointMillis >= startMillis {
				break
			}
		}
		// Slice the array starting from cutoff index
		metric.MetricPoints = points[cutoff:]
	}
	return mf
}

// applySlidingWindow applies a given processing function over a moving window of metric points.
// This is the core generic function that contains the shared logic.
func applySlidingWindow(mf *metrics.MetricFamily, lookback int, processor func(window []*metrics.MetricPoint) float64) *metrics.MetricFamily {
	for _, metric := range mf.Metrics {
		points := metric.MetricPoints
		if len(points) == 0 {
			continue
		}

		processedPoints := make([]*metrics.MetricPoint, 0, len(points))

		for i, currentPoint := range points {
			// Define the start of the moving window, ensuring it's not out of bounds.
			start := i - lookback + 1
			if start < 0 {
				start = 0
			}
			window := points[start : i+1]

			// Delegate the specific calculation to the provided processor function.
			resultValue := processor(window)

			processedPoints = append(processedPoints, &metrics.MetricPoint{
				Timestamp: currentPoint.Timestamp,
				Value:     toDomainMetricPointValue(resultValue),
			})
		}
		metric.MetricPoints = processedPoints
	}
	return mf
}

// calcCallRate defines the rate calculation logic and pass in applySlidingWindow.
func calcCallRate(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters) *metrics.MetricFamily {
	lookback := int(math.Ceil(float64(params.RatePer.Milliseconds()) / float64(params.Step.Milliseconds())))
	// Ensure lookback >= 1
	lookback = int(math.Max(float64(lookback), 1))

	windowSizeSeconds := float64(lookback) * params.Step.Seconds()

	// rateCalculator is a closure that captures 'lookback' and 'windowSizeSeconds'.
	// It implements the specific logic for calculating the rate.
	rateCalculator := func(window []*metrics.MetricPoint) float64 {
		// If the window is not "full" (i.e., we don't have enough preceding points
		// to calculate a rate over the full 'lookback' period), return NaN.
		if len(window) < lookback {
			return math.NaN()
		}

		firstValue := window[0].GetGaugeValue().GetDoubleValue()
		// If the first value in the full window is NaN, treat it as 0 for the rate calculation.
		// This implies that if data was missing at the start of the window, we assume no contribution from that missing period.
		if math.IsNaN(firstValue) {
			firstValue = 0
		}
		lastValue := window[len(window)-1].GetGaugeValue().GetDoubleValue()
		// If the current point (the last value in the window) is NaN, the rate cannot be defined.
		// Propagate NaN to indicate missing data for the result point.
		if math.IsNaN(lastValue) {
			return math.NaN()
		}

		rate := (lastValue - firstValue) / windowSizeSeconds
		return math.Round(rate*100) / 100
	}

	return applySlidingWindow(mf, lookback, rateCalculator)
}

func scaleToMillisAndRound(window []*metrics.MetricPoint) float64 {
	if len(window) == 0 {
		return math.NaN()
	}

	v := window[len(window)-1].GetGaugeValue().GetDoubleValue()
	// Scale down the value (e.g., from microseconds to milliseconds)
	resultValue := v / 1000.0
	return math.Round(resultValue*100) / 100 // Round to 2 decimal places
}

// bucketsToPoints is a helper function for getting points value from ES AGG bucket
func bucketsToPoints(buckets []*elastic.AggregationBucketHistogramItem, valueExtractor func(*elastic.AggregationBucketHistogramItem) float64) []*Pair {
	var points []*Pair

	for _, bucket := range buckets {
		var value float64
		// If there is no data (doc_count = 0), we return NaN()
		if bucket.DocCount == 0 {
			value = math.NaN()
		} else {
			// Else extract the value and return it
			value = valueExtractor(bucket)
		}

		points = append(points, &Pair{
			TimeStamp: int64(bucket.Key),
			Value:     value,
		})
	}
	return points
}

func bucketsToCallRate(buckets []*elastic.AggregationBucketHistogramItem) []*Pair {
	valueExtractor := func(bucket *elastic.AggregationBucketHistogramItem) float64 {
		aggMap, ok := bucket.Aggregations.CumulativeSum(culmuAggName)
		if !ok || aggMap.Value == nil {
			return math.NaN()
		}
		return *aggMap.Value
	}
	return bucketsToPoints(buckets, valueExtractor)
}

func bucketsToLatencies(buckets []*elastic.AggregationBucketHistogramItem, percentileValue float64) []*Pair {
	valueExtractor := func(bucket *elastic.AggregationBucketHistogramItem) float64 {
		aggMap, ok := bucket.Aggregations.Percentiles(percentilesAggName)
		if !ok {
			return math.NaN()
		}
		percentileKey := fmt.Sprintf("%.1f", percentileValue)
		aggMapValue, ok := aggMap.Values[percentileKey]
		if !ok {
			return math.NaN()
		}
		return aggMapValue
	}
	return bucketsToPoints(buckets, valueExtractor)
}

// executeSearch performs the Elasticsearch search.
func (r MetricsReader) executeSearch(ctx context.Context, p MetricsQueryParams) (*elastic.SearchResult, error) {
	span := r.queryLogger.TraceQuery(ctx, p.metricName)
	defer span.End()

	searchResult, err := r.queryBuilder.Execute(ctx, p.boolQuery, p.aggQuery)
	if err != nil {
		err = fmt.Errorf("failed executing metrics query: %w", err)
		r.queryLogger.LogErrorToSpan(span, err)
		return nil, err
	}

	r.queryLogger.LogAndTraceResult(span, searchResult)

	// Return raw search result
	return searchResult, nil
}

func calculateTimeRange(params *metricstore.BaseQueryParameters) (TimeRange, error) {
	if params == nil || params.EndTime == nil || params.Lookback == nil {
		return TimeRange{}, errors.New("invalid parameters")
	}
	endTime := *params.EndTime
	startTime := endTime.Add(-*params.Lookback)
	extendedStartTime := startTime.Add(-10 * time.Minute)

	return TimeRange{
		startTimeMillis:         startTime.UnixMilli(),
		endTimeMillis:           endTime.UnixMilli(),
		extendedStartTimeMillis: extendedStartTime.UnixMilli(),
	}, nil
}
