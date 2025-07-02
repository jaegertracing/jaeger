// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/olivere/elastic/v7"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

var ErrNotImplemented = errors.New("metrics querying is currently not implemented yet")

const (
	minStep            = time.Millisecond
	aggName            = "results_buckets"
	culmuAggName       = "cumulative_requests"
	percentilesAggName = "percentiles_of_bucket"
	dateHistAggName    = "date_histogram"
)

// MetricsReader is an Elasticsearch metrics reader.
type MetricsReader struct {
	client      es.Client
	cfg         config.Configuration
	logger      *zap.Logger
	tracer      trace.Tracer
	queryLogger *QueryLogger
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
	// bucketsToPointsFunc is a function that turn raw ES Histogram Bucket result into
	// array of Pair for easier post-processing (using processMetricsFunc)
	bucketsToPointsFunc func(buckets []*elastic.AggregationBucketHistogramItem) []*Pair
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
		client:      client,
		cfg:         cfg,
		logger:      logger,
		tracer:      tr,
		queryLogger: NewQueryLogger(logger, tr),
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
		boolQuery:           r.buildQuery(params.BaseQueryParameters, timeRange),
		aggQuery:            r.buildLatenciesAggregations(params, timeRange),
		bucketsToPointsFunc: func(buckets []*elastic.AggregationBucketHistogramItem) []*Pair {
			return getLatenciesBucketsToPoints(buckets, params.Quantile*100)
		},
	}

	rawMetricFamily, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}

	// Process the raw aggregation value to calculate call_rate (req/s)
	return getLatenciesProcessMetrics(rawMetricFamily, *params), nil
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
		boolQuery:           r.buildQuery(params.BaseQueryParameters, timeRange),
		aggQuery:            r.buildCallRateAggregations(params.BaseQueryParameters, timeRange),
		bucketsToPointsFunc: getCallRateBucketsToPoints,
	}

	rawMetricFamily, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}

	// Process the raw aggregation value to calculate call_rate (req/s)
	processedMetricFamily := getCallRateProcessMetrics(rawMetricFamily, params.BaseQueryParameters)
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

// buildQuery constructs the Elasticsearch bool query.
func (r MetricsReader) buildQuery(params metricstore.BaseQueryParameters, timeRange TimeRange) elastic.BoolQuery {
	boolQuery := elastic.NewBoolQuery()

	serviceNameQuery := elastic.NewTermsQuery("process.serviceName", buildInterfaceSlice(params.ServiceNames)...)
	boolQuery.Filter(serviceNameQuery)

	// Span kind filter
	spanKindField := strings.ReplaceAll(model.SpanKindKey, ".", r.cfg.Tags.DotReplacement)
	spanKindQuery := elastic.NewTermsQuery("tag."+spanKindField, buildInterfaceSlice(normalizeSpanKinds(params.SpanKinds))...)
	boolQuery.Filter(spanKindQuery)

	rangeQuery := elastic.NewRangeQuery("startTimeMillis").
		// Use extendedStartTimeMillis to allow for a 10-minute lookback.
		Gte(timeRange.extendedStartTimeMillis).
		Lte(timeRange.endTimeMillis).
		Format("epoch_millis")
	boolQuery.Filter(rangeQuery)

	// Corresponding ES query:
	// {
	// "query": {
	//	"bool": {
	//		"filter": [
	//			{"terms": {"process.serviceName": ["name1"] }},
	//			{"terms": {"tag.span@kind": ["server"] }}, // Dot replacement: @
	//			{
	//			"range": {
	//			"startTimeMillis": {
	//				"gte": "now-'lookback'-5m",
	//				"lte": "now",
	//				"format": "epoch_millis"}}}]}
	// },

	return *boolQuery
}

// processCallRateMetrics processes the MetricFamily to calculate call rates
func getCallRateProcessMetrics(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters) *metrics.MetricFamily {
	lookback := int(math.Ceil(float64(params.RatePer.Milliseconds()) / float64(params.Step.Milliseconds())))
	if lookback < 1 {
		lookback = 1
	}

	for _, metric := range mf.Metrics {
		points := metric.MetricPoints
		var processedPoints []*metrics.MetricPoint

		for i := range points {
			currentPoint := points[i]
			currentValue := currentPoint.GetGaugeValue().GetDoubleValue()

			// Elasticsearch's percentiles aggregation returns 0.0 for time buckets with no documents
			// These aren't true zero values but represent missing data points in sparse time series
			// We convert them to NaN to distinguish from actual measured zero values (slope of 0)
			if currentValue == 0.0 {
				processedPoints = append(processedPoints, &metrics.MetricPoint{
					Timestamp: currentPoint.Timestamp,
					Value:     toDomainMetricPointValue(math.NaN()),
				})
				continue
			}

			// For first (lookback-1) points, we don't have enough history
			if i < lookback-1 {
				processedPoints = append(processedPoints, &metrics.MetricPoint{
					Timestamp: currentPoint.Timestamp,
					Value:     toDomainMetricPointValue(0.0),
				})
				continue
			}

			// Get boundary values for our lookback window
			firstPoint := points[i-lookback+1]
			firstValue := firstPoint.GetGaugeValue().GetDoubleValue()
			lastValue := currentValue

			// Calculate time window duration in seconds
			windowSizeSeconds := float64(lookback) * params.Step.Seconds()

			// Calculate rate of change per second
			rate := (lastValue - firstValue) / windowSizeSeconds
			rate = math.Round(rate*100) / 100 // Round to 2 decimal places

			processedPoints = append(processedPoints, &metrics.MetricPoint{
				Timestamp: currentPoint.Timestamp,
				Value:     toDomainMetricPointValue(rate),
			})
		}

		metric.MetricPoints = processedPoints
	}

	return mf
}

func getLatenciesProcessMetrics(mf *metrics.MetricFamily, params metricstore.LatenciesQueryParameters) *metrics.MetricFamily {
	// Configuration - sliding window size (10 data points)
	window := int(math.Ceil(float64(params.RatePer.Milliseconds()) / float64(params.Step.Milliseconds())))
	if window < 1 {
		window = 1
	}

	for _, metric := range mf.Metrics {
		points := metric.MetricPoints
		processedPoints := make([]*metrics.MetricPoint, 0, len(points))

		// Process each data point in the time series
		for i := range points {
			currentPoint := points[i]
			currentValue := currentPoint.GetGaugeValue().GetDoubleValue()

			// Elasticsearch's percentiles aggregation returns 0.0 for time buckets with no documents
			// These aren't true zero values but represent missing data points in sparse time series
			// We convert them to NaN to distinguish from actual measured zero values
			if currentValue == 0.0 {
				processedPoints = append(processedPoints, &metrics.MetricPoint{
					Timestamp: currentPoint.Timestamp,
					Value:     toDomainMetricPointValue(math.NaN()),
				})
				continue
			}

			// Calculate window boundaries (ensuring we don't go before start of data)
			start := max(0, i-window+1)

			// Collect all non-NaN values within our window
			var valid []float64
			for j := start; j <= i; j++ {
				if v := points[j].GetGaugeValue().GetDoubleValue(); !math.IsNaN(v) {
					valid = append(valid, v)
				}
			}

			// If no valid values in window, return NaN for this point
			if len(valid) == 0 {
				processedPoints = append(processedPoints, &metrics.MetricPoint{
					Timestamp: currentPoint.Timestamp,
					Value:     toDomainMetricPointValue(math.NaN()),
				})
				continue
			}

			// Sort values to enable accurate percentile calculation
			sort.Float64s(valid)

			// Calculate index for desired percentile
			// params.Quantile is the target percentile (e.g., 0.95 for 95th)
			idx := int(math.Ceil(params.Quantile * float64(len(valid)-1)))
			// Scale down the result value (division by 1000 to get milliseconds)
			resultValue := valid[idx] / 1000.0

			// Store the calculated percentile value with original timestamp
			processedPoints = append(processedPoints, &metrics.MetricPoint{
				Timestamp: currentPoint.Timestamp,
				Value:     toDomainMetricPointValue(resultValue),
			})
		}

		metric.MetricPoints = processedPoints
	}

	return mf
}

func getCallRateBucketsToPoints(buckets []*elastic.AggregationBucketHistogramItem) []*Pair {
	var points []*Pair

	for _, bucket := range buckets {
		aggMap, ok := bucket.Aggregations.CumulativeSum(culmuAggName)
		if !ok {
			return nil
		}
		value := math.NaN()
		if aggMap != nil && aggMap.Value != nil {
			value = *aggMap.Value
		}
		points = append(points, &Pair{
			TimeStamp: int64(bucket.Key),
			Value:     value,
		})
	}
	return points
}

func getLatenciesBucketsToPoints(buckets []*elastic.AggregationBucketHistogramItem, percentileValue float64) []*Pair {
	var points []*Pair

	// Process each bucket from the Elasticsearch histogram aggregation
	for _, bucket := range buckets {
		// Extract percentiles aggregation from the bucket
		aggMap, ok := bucket.Aggregations.Percentiles(percentilesAggName)
		if !ok {
			return nil
		}

		// Format the percentile key to match Elasticsearch's format (e.g., "95.0")
		percentileKey := fmt.Sprintf("%.1f", percentileValue)
		aggMapValue, ok := aggMap.Values[percentileKey]
		if !ok {
			return nil
		}

		value := math.NaN()
		if !math.IsNaN(aggMapValue) {
			value = aggMapValue
		}

		points = append(points, &Pair{
			TimeStamp: int64(bucket.Key),
			Value:     value,
		})
	}
	return points
}

func (MetricsReader) buildTimeSeriesAggregation(params metricstore.BaseQueryParameters, timeRange TimeRange, subAggName string, subAgg elastic.Aggregation) elastic.Aggregation {
	fixedIntervalString := strconv.FormatInt(params.Step.Milliseconds(), 10) + "ms"

	dateHistAgg := elastic.NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval(fixedIntervalString).
		MinDocCount(0).
		ExtendedBounds(timeRange.extendedStartTimeMillis, timeRange.endTimeMillis)

	dateHistAgg = dateHistAgg.SubAggregation(subAggName, subAgg)

	if params.GroupByOperation {
		return elastic.NewTermsAggregation().
			Field("operationName").
			Size(10).
			SubAggregation(dateHistAggName, dateHistAgg)
	}

	return dateHistAgg
}

// buildCallRateAggregations now calls the generic builder function.
func (r MetricsReader) buildCallRateAggregations(params metricstore.BaseQueryParameters, timeRange TimeRange) elastic.Aggregation {
	cumulativeSumAgg := elastic.NewCumulativeSumAggregation().BucketsPath("_count")

	return r.buildTimeSeriesAggregation(params, timeRange, culmuAggName, cumulativeSumAgg)
}

// buildLatenciesAggregations now calls the generic builder function.
func (r MetricsReader) buildLatenciesAggregations(params *metricstore.LatenciesQueryParameters, timeRange TimeRange) elastic.Aggregation {
	percentileValue := params.Quantile * 100
	percentilesAgg := elastic.NewPercentilesAggregation().
		Field("duration").
		Percentiles(percentileValue)

	return r.buildTimeSeriesAggregation(params.BaseQueryParameters, timeRange, percentilesAggName, percentilesAgg)
}

// executeSearch performs the Elasticsearch search.
func (r MetricsReader) executeSearch(ctx context.Context, p MetricsQueryParams) (*metrics.MetricFamily, error) {
	if p.GroupByOperation {
		p.metricName = strings.Replace(p.metricName, "service", "service_operation", 1)
		p.metricDesc += " & operation"
	}

	span := r.queryLogger.TraceQuery(ctx, p.metricName)
	defer span.End()

	indexName := r.cfg.Indices.IndexPrefix.Apply("jaeger-span-*")
	searchResult, err := r.client.Search(indexName).
		Query(&p.boolQuery).
		Size(0). // Set Size to 0 to return only aggregation results, excluding individual search hits
		Aggregation(aggName, p.aggQuery).
		Do(ctx)
	if err != nil {
		err = fmt.Errorf("failed executing metrics query: %w", err)
		r.queryLogger.LogErrorToSpan(span, err)
		return nil, err
	}

	r.queryLogger.LogAndTraceResult(span, searchResult)

	// Return raw result
	return ToDomainMetricsFamily(p, searchResult)
}

// normalizeSpanKinds normalizes a slice of span kinds.
func normalizeSpanKinds(spanKinds []string) []string {
	normalized := make([]string, len(spanKinds))
	for i, kind := range spanKinds {
		normalized[i] = strings.ToLower(strings.TrimPrefix(kind, "SPAN_KIND_"))
	}
	return normalized
}

// buildInterfaceSlice converts []string to []interface{} for elastic terms query.
func buildInterfaceSlice(s []string) []any {
	ifaceSlice := make([]any, len(s))
	for i, v := range s {
		ifaceSlice[i] = v
	}
	return ifaceSlice
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
