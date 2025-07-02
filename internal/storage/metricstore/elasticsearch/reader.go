// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	minStep      = time.Millisecond
	aggName      = "results_buckets"
	searchIndex  = "jaeger-span-*"
	culmuAggName = "cumulative_requests"
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
	// processMetricsFunc is a function that processes the raw time-series
	// data (pairs of timestamp and value) returned from Elasticsearch
	// aggregations into the final metric values. This is used for calculations
	// like rates (e.g., calls/sec) which require manipulating the raw counts
	// or sums over specific time windows.
	processMetricsFunc func(pair []*Pair, params metricstore.BaseQueryParameters) []*Pair
}

// Pair represents a timestamp-value pair for metrics.
type Pair struct {
	TimeStamp int64
	Value     float64
}

func newPair(ts int64, value float64) *Pair {
	return &Pair{
		TimeStamp: ts,
		Value:     value,
	}
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
func (MetricsReader) GetLatencies(_ context.Context, _ *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
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
		processMetricsFunc:  getCallRateProcessMetrics,
	}

	metricFamily, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}
	// Trim results to original time range
	return trimMetricPointsBefore(metricFamily, timeRange.startTimeMillis), nil
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

func getCallRateProcessMetrics(pairs []*Pair, m metricstore.BaseQueryParameters) []*Pair {
	lookback := int(math.Ceil(float64(m.RatePer.Milliseconds()) / float64(m.Step.Milliseconds())))
	if lookback < 1 {
		lookback = 1 // Ensure we always have at least 1 point
	}
	n := len(pairs)
	results := make([]*Pair, 0, n) // Pre-allocate result slice for efficiency

	for i := range pairs {
		// Elasticsearch's percentiles aggregation returns 0.0 for time buckets with no documents
		// These aren't true zero values but represent missing data points in sparse time series
		// We convert them to NaN to distinguish from actual measured zero values (slope of 0)
		if pairs[i].Value == 0.0 {
			results = append(results, newPair(pairs[i].TimeStamp, math.NaN()))
			continue
		}

		// For first (lookback-1) points, we don't have enough history
		if i < lookback-1 {
			results = append(results, newPair(pairs[i].TimeStamp, 0.0))
			continue
		}

		// Get boundary values for our lookback window:
		// First value in window (oldest)
		firstVal := pairs[i-lookback+1].Value
		// Last value in window (current value)
		lastVal := pairs[i].Value

		// Calculate time window duration in seconds
		// params.Step.Seconds() gives the interval between data points
		windowSizeSeconds := float64(lookback) * (m.Step.Seconds())

		// Calculate rate of change per second
		// Formula: (current_value - starting_value) / time_window
		rate := (lastVal - firstVal) / windowSizeSeconds

		// Store the result with original timestamp
		results = append(results, newPair(pairs[i].TimeStamp, rate))
	}

	return results
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

// buildCallRateAggregations constructs the GetCallRate aggregations.
func (MetricsReader) buildCallRateAggregations(params metricstore.BaseQueryParameters, timeRange TimeRange) elastic.Aggregation {
	fixedIntervalString := strconv.FormatInt(params.Step.Milliseconds(), 10) + "ms"
	dateHistoAgg := elastic.NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval(fixedIntervalString).
		MinDocCount(0).
		ExtendedBounds(timeRange.extendedStartTimeMillis, timeRange.endTimeMillis)

	cumulativeSumAgg := elastic.NewCumulativeSumAggregation().BucketsPath("_count")

	// Corresponding AGG ES query:
	// "aggs": {
	//	"results_buckets": {
	//		"date_histogram": {
	//			"field": "startTimeMillis",
	//				"fixed_interval": "60s",
	//				"min_doc_count": 0,
	//				"extended_bounds": {
	//				"min": "now-lookback-5m",
	//				"max": "now"
	//			}
	//		},
	//		"aggs": {
	//			"cumulative_requests": {
	//				"cumulative_sum": {
	//					"buckets_path": "_count"
	//				}
	//			}
	//

	dateHistoAgg = dateHistoAgg.
		SubAggregation(culmuAggName, cumulativeSumAgg)

	if params.GroupByOperation {
		operationsAgg := elastic.NewTermsAggregation().
			Field("operationName").
			Size(10).
			SubAggregation("date_histogram", dateHistoAgg) // Nest the dateHistoAgg inside operationsAgg
		return operationsAgg
	}

	return dateHistoAgg
}

// executeSearch performs the Elasticsearch search.
func (r MetricsReader) executeSearch(ctx context.Context, p MetricsQueryParams) (*metrics.MetricFamily, error) {
	if p.GroupByOperation {
		p.metricName = strings.Replace(p.metricName, "service", "service_operation", 1)
		p.metricDesc += " & operation"
	}

	// Use the QueryLogger for logging and tracing the query
	span := r.queryLogger.TraceQuery(ctx, p.metricName)
	defer span.End()

	searchResult, err := r.client.Search(searchIndex).
		Query(&p.boolQuery).
		Size(0). // Set Size to 0 to return only aggregation results, excluding individual search hits
		Aggregation(aggName, p.aggQuery).
		Do(ctx)
	if err != nil {
		err = fmt.Errorf("failed executing metrics query: %w", err)
		r.queryLogger.LogErrorToSpan(span, err) // Use the QueryLogger for logging error to span
		return nil, err
	}

	// Use the QueryLogger for logging and tracing the results
	r.queryLogger.LogAndTraceResult(span, searchResult)

	return ToDomainMetricsFamily(
		p,
		searchResult,
	)
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
