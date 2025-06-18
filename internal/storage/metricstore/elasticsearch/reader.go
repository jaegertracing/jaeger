// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	elasticv7 "github.com/olivere/elastic/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

var ErrNotImplemented = errors.New("metrics querying is currently not implemented yet")

const (
	minStep      = time.Millisecond
	movFnAggName = "results"
	aggName      = "results_buckets"
	searchIndex  = "jaeger-span-*"
)

// MetricsReader is an Elasticsearch metrics reader.
type (
	MetricsReader struct {
		client            es.Client
		logger            *zap.Logger
		tracer            trace.Tracer
		metricsTranslator Translator
	}
	TimeRange struct {
		startTimeMillis         int64
		endTimeMillis           int64
		extendedStartTimeMillis int64
	}

	MetricsQueryParams struct {
		metricstore.BaseQueryParameters
		metricName string
		metricDesc string
		boolQuery  *elasticv7.BoolQuery
		aggQuery   elasticv7.Aggregation
	}
)

// NewMetricsReader initializes a new MetricsReader.
func NewMetricsReader(client es.Client, logger *zap.Logger, tracer trace.TracerProvider) *MetricsReader {
	return &MetricsReader{
		client:            client,
		logger:            logger,
		tracer:            tracer.Tracer("elasticsearch-metricstore"),
		metricsTranslator: NewTranslator(),
	}
}

// GetLatencies retrieves latency metrics
func (MetricsReader) GetLatencies(_ context.Context, _ *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

// GetCallRates retrieves call rate metrics
func (r MetricsReader) GetCallRates(ctx context.Context, params *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	timeRange := calculateTimeRange(&params.BaseQueryParameters)

	metricsParams := MetricsQueryParams{
		BaseQueryParameters: params.BaseQueryParameters,
		metricName:          "service_call_rate",
		metricDesc:          "calls/sec, grouped by service",
		boolQuery:           r.buildQuery(&params.BaseQueryParameters, timeRange),
		aggQuery:            r.buildCallRateAggregations(params, timeRange),
	}

	metricFamily, err := r.executeSearch(ctx, metricsParams)
	if err != nil {
		return nil, err
	}

	// Trim results to original time range
	if metricFamily != nil {
		metricFamily = trimMetricPointsBefore(metricFamily, timeRange.startTimeMillis)
	}
	return metricFamily, nil
}

// GetErrorRates retrieves error rate metrics
func (MetricsReader) GetErrorRates(_ context.Context, _ *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

// GetMinStepDuration returns the minimum step duration.
func (MetricsReader) GetMinStepDuration(_ context.Context, _ *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
}

// Add this helper method
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
func (MetricsReader) buildQuery(params *metricstore.BaseQueryParameters, timeRange TimeRange) *elasticv7.BoolQuery {
	boolQuery := elasticv7.NewBoolQuery()

	serviceNameQuery := elasticv7.NewTermsQuery("process.serviceName", buildInterfaceSlice(params.ServiceNames)...)
	boolQuery.Filter(serviceNameQuery)

	// Span kind filter
	spanKindQuery := buildSpanKindQuery(params.SpanKinds)
	nestedTagsQuery := elasticv7.NewNestedQuery("tags",
		elasticv7.NewBoolQuery().
			Must(
				elasticv7.NewTermQuery("tags.key", "span.kind"),
				spanKindQuery,
			),
	)
	boolQuery.Filter(nestedTagsQuery)

	rangeQuery := elasticv7.NewRangeQuery("startTimeMillis").
		// Use extendedStartTimeMillis to allow for a 5-minute lookback.
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
	//			{
	//			"nested": {
	//				"path": "tags",
	//				"query": {
	//					"bool": {
	//						"must": [
	//						{"term": {"tags.key": "span.kind"}},
	//						{"term": {"tags.value": "server"}}]}}}},
	//			{
	//			"range": {
	//			"startTimeMillis": {
	//				"gte": "now-'lookback'-5m",
	//				"lte": "now",
	//				"format": "epoch_millis"}}}]}
	// },

	return boolQuery
}

// buildSpanKindQuery constructs the query for span kinds.
func buildSpanKindQuery(spanKinds []string) elasticv7.Query {
	querySpanKinds := normalizeSpanKinds(spanKinds)
	if len(querySpanKinds) == 1 {
		return elasticv7.NewTermQuery("tags.value", querySpanKinds[0])
	}

	shouldQuery := elasticv7.NewBoolQuery()
	for _, kind := range querySpanKinds {
		shouldQuery.Should(elasticv7.NewTermQuery("tags.value", kind))
	}
	return shouldQuery
}

// buildCallRateAggregations constructs the GetCallRate aggregations.
func (MetricsReader) buildCallRateAggregations(params *metricstore.CallRateQueryParameters, timeRange TimeRange) elasticv7.Aggregation {
	fixedIntervalString := strconv.FormatInt(params.Step.Milliseconds(), 10) + "ms"
	dateHistoAgg := elasticv7.NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval(fixedIntervalString).
		MinDocCount(0).
		ExtendedBounds(timeRange.startTimeMillis, timeRange.endTimeMillis)

	cumulativeSumAgg := elasticv7.NewCumulativeSumAggregation().BucketsPath("_count")

	// Painless script to calculate the rate per second using linear regression.
	painlessScriptSource := `
		if (values == null || values.length == 0) return 0.0;
		if (values.length < 2) return 0.0;
		double n = values.length;
		double sumX = 0.0;
		double sumY = 0.0;
		double sumXY = 0.0;
		double sumX2 = 0.0;
		for (int i = 0; i < n; i++) {
			double x = i;
			double y = values[i];
			sumX += x;
			sumY += y;
			sumXY += x * y;
			sumX2 += x * x;
		}
		double numerator = n * sumXY - sumX * sumY;
		double denominator = n * sumX2 - sumX * sumX;
		if (Math.abs(denominator) < 1e-10) return 0.0;
		double slopePerBucket = numerator / denominator;
		double intervalSeconds = params.interval_ms / 1000.0;
		return slopePerBucket / intervalSeconds;
    `
	scriptParams := map[string]any{
		"window":      10,
		"interval_ms": params.Step.Milliseconds(),
	}
	movingFnAgg := (&elasticv7.MovFnAggregation{}).
		BucketsPath("cumulative_requests").
		Script(elasticv7.NewScript(painlessScriptSource).Lang("painless").Params(scriptParams)).
		Window(10)

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
	//			},
	//			"rate_per_second": {
	//				"moving_fn": {
	//					"script": {
	//						"source": scriptSource,
	//							"lang": "painless",
	//							"params": {
	//							"window": 10,
	//								"interval_ms": 60000
	//						}
	//					},
	//					"buckets_path": "cumulative_requests",
	//						"window": 10}}}}}

	dateHistoAgg = dateHistoAgg.
		SubAggregation("cumulative_requests", cumulativeSumAgg).
		SubAggregation(movFnAggName, movingFnAgg)

	if params.GroupByOperation {
		operationsAgg := elasticv7.NewTermsAggregation().
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

	queryString, err := r.buildQueryJSON(p.boolQuery, p.aggQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to build query JSON: %w", err)
	}

	span := startSpanForQuery(ctx, p.metricName, queryString, r.tracer)
	defer span.End()

	searchResult, err := r.client.Search(searchIndex).
		Query(p.boolQuery).
		Size(0).
		Aggregation(aggName, p.aggQuery).
		Do(ctx)
	if err != nil {
		err = fmt.Errorf("failed executing metrics query: %w", err)
		logErrorToSpan(span, err)
		return &metrics.MetricFamily{}, err
	}

	result, _ := json.MarshalIndent(searchResult, "", "  ")

	r.logger.Debug("Elasticsearch metricsreader query results", zap.String("results", string(result)), zap.String("query", queryString))

	return r.metricsTranslator.ToMetricsFamily(
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

// calculateTimeRange computes the time range for the query.
func calculateTimeRange(params *metricstore.BaseQueryParameters) TimeRange {
	endTime := *params.EndTime
	startTime := endTime.Add(-*params.Lookback)
	extendedStartTime := startTime.Add(-5 * time.Minute)

	return TimeRange{
		startTimeMillis:         startTime.UnixMilli(),
		endTimeMillis:           endTime.UnixMilli(),
		extendedStartTimeMillis: extendedStartTime.UnixMilli(),
	}
}

func (MetricsReader) buildQueryJSON(boolQuery *elasticv7.BoolQuery, aggQuery elasticv7.Aggregation) (string, error) {
	// Combine query and aggregations into a search source
	searchSource := elasticv7.NewSearchSource().
		Query(boolQuery).
		Aggregation(aggName, aggQuery).
		Size(0)

	source, err := searchSource.Source()
	if err != nil {
		return "", fmt.Errorf("failed to get query source: %w", err)
	}

	queryJSON, _ := json.MarshalIndent(source, "", "  ")

	return string(queryJSON), nil
}

func startSpanForQuery(ctx context.Context, metricName, query string, tp trace.Tracer) trace.Span {
	_, span := tp.Start(ctx, metricName)
	span.SetAttributes(
		otelsemconv.DBQueryTextKey.String(query),
		otelsemconv.DBSystemKey.String("elasticsearch"),
		attribute.Key("component").String("es-metricsreader"),
	)
	return span
}

func logErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
