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
	return ProcessLatencies(rawMetricFamily), nil
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

	return ProcessCallRates(rawMetricFamily, params.BaseQueryParameters, timeRange), nil
}

// GetErrorRates retrieves error rate metrics
func (r MetricsReader) GetErrorRates(ctx context.Context, params *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	timeRange, err := calculateTimeRange(&params.BaseQueryParameters)
	if err != nil {
		return nil, err
	}

	metricsParams := MetricsQueryParams{
		BaseQueryParameters: params.BaseQueryParameters,
		metricName:          "service_error_rate",
		metricDesc:          "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
		boolQuery:           r.queryBuilder.BuildErrorBoolQuery(params.BaseQueryParameters, timeRange),
		aggQuery:            r.queryBuilder.BuildCallRateAggQuery(params.BaseQueryParameters, timeRange), // Use the same aggQuery as GetCallRates
	}

	searchResult, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}
	// Convert search results into raw metric family using translator
	translator := NewTranslator(bucketsToCallRate)
	rawErrorsMetrics, err := translator.ToDomainMetricsFamily(metricsParams, searchResult)
	if err != nil {
		return nil, err
	}

	callRateMetrics, err := r.GetCallRates(ctx, &metricstore.CallRateQueryParameters{BaseQueryParameters: params.BaseQueryParameters})
	if err != nil {
		return nil, err
	}

	return ProcessErrorRates(rawErrorsMetrics, callRateMetrics, params.BaseQueryParameters, timeRange), nil
}

// GetMinStepDuration returns the minimum step duration.
func (MetricsReader) GetMinStepDuration(_ context.Context, _ *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
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
