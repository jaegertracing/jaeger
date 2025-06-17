// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/elasticsearch/translator"
	"strconv"
	"strings"
	"time"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	elasticv7 "github.com/olivere/elastic/v7"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

var ErrNotImplemented = errors.New("metrics querying is currently not implemented yet")

const (
	minStep         = time.Millisecond
	movFnAggName    = "results"
	dateHistAggName = "results_buckets"
	searchIndex     = "jaeger-span-*"
)

// MetricsReader is an Elasticsearch metrics reader.
type (
	MetricsReader struct {
		client            es.Client
		logger            *zap.Logger
		tracer            trace.Tracer
		metricsTranslator translator.Translator
	}
	TimeRange struct {
		startTimeMillis         int64
		endTimeMillis           int64
		extendedStartTimeMillis int64
	}

	MetricsQueryParams struct {
		metricstore.BaseQueryParameters
		metricName   string
		metricDesc   string
		boolQuery    *elasticv7.BoolQuery
		dateHistoAgg *elasticv7.DateHistogramAggregation
	}
)

// NewMetricsReader initializes a new MetricsReader.
func NewMetricsReader(client es.Client, logger *zap.Logger, tracer trace.TracerProvider) *MetricsReader {
	return &MetricsReader{
		client:            client,
		logger:            logger,
		tracer:            tracer.Tracer("elasticsearch-metricstore"),
		metricsTranslator: translator.New("span_name"),
	}, nil
}

// GetLatencies retrieves latency metrics by delegating to GetCallRates.
func (r MetricsReader) GetLatencies(_ context.Context, _ *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

// GetCallRates retrieves call rate metrics from Elasticsearch.
func (r MetricsReader) GetCallRates(ctx context.Context, params *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	timeRange, err := r.calculateTimeRange(&params.BaseQueryParameters)
	if err != nil {
		return nil, err
	}

	metricsParams := MetricsQueryParams{
		BaseQueryParameters: params.BaseQueryParameters,
		metricName:          "service_call_rate",
		metricDesc:          "calls/sec, grouped by service",
		boolQuery:           r.buildQuery(&params.BaseQueryParameters, timeRange),
		dateHistoAgg:        r.buildCallRateAggregations(params, timeRange),
	}

	metricFamily, err := r.executeSearch(ctx, metricsParams, false)
	if err != nil {
		return nil, err
	}

	// Trim results to original time range
	if metricFamily != nil {
		metricFamily = r.trimMetricPointsBefore(metricFamily, timeRange.startTimeMillis)
	}
	return metricFamily, nil

}

// GetErrorRates retrieves error rate metrics by delegating to GetCallRates.
func (r MetricsReader) GetErrorRates(_ context.Context, _ *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, ErrNotImplemented
}

// GetMinStepDuration returns the minimum step duration.
func (r MetricsReader) GetMinStepDuration(_ context.Context, _ *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
}

// Add this helper method
func (r MetricsReader) trimMetricPointsBefore(mf *metrics.MetricFamily, startMillis int64) *metrics.MetricFamily {
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
func (r MetricsReader) buildQuery(params *metricstore.BaseQueryParameters, timeRange TimeRange) *elasticv7.BoolQuery {
	boolQuery := elasticv7.NewBoolQuery()

	serviceNameQuery := elasticv7.NewTermsQuery("process.serviceName", buildInterfaceSlice(params.ServiceNames)...)
	boolQuery.Filter(serviceNameQuery)

	// Span kind filter
	spanKindQuery := r.buildSpanKindQuery(params.SpanKinds)
	nestedTagsQuery := elasticv7.NewNestedQuery("tags",
		elasticv7.NewBoolQuery().
			Must(
				elasticv7.NewTermQuery("tags.key", "span.kind"),
				spanKindQuery,
			),
	)
	boolQuery.Filter(nestedTagsQuery)

	rangeQuery := elasticv7.NewRangeQuery("startTimeMillis").
		Gte(timeRange.extendedStartTimeMillis).
		Lte(timeRange.endTimeMillis).
		Format("epoch_millis")
	boolQuery.Filter(rangeQuery)

	return boolQuery
}

// buildSpanKindQuery constructs the query for span kinds.
func (r MetricsReader) buildSpanKindQuery(spanKinds []string) elasticv7.Query {
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

// buildAggregations constructs the getcallrate aggregations.
func (r MetricsReader) buildCallRateAggregations(params *metricstore.CallRateQueryParameters, timeRange TimeRange) *elasticv7.DateHistogramAggregation {
	fixedIntervalString := strconv.FormatInt(params.Step.Milliseconds(), 10) + "ms"
	dateHistoAgg := elasticv7.NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval(fixedIntervalString).
		MinDocCount(0).
		ExtendedBounds(timeRange.startTimeMillis, timeRange.endTimeMillis)
	r.logger.Info("Date histogram aggregation built", zap.String("fixedInterval", fixedIntervalString))

	cumulativeSumAgg := elasticv7.NewCumulativeSumAggregation().BucketsPath("_count")
	r.logger.Info("Cumulative sum aggregation built")

	painlessScriptSource := `
		if (values == null || values.length < 2) return 0.0;
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
	scriptParams := map[string]interface{}{
		"window":      10,
		"interval_ms": params.Step.Milliseconds(),
	}
	movingFnAgg := (&elasticv7.MovFnAggregation{}).
		BucketsPath("cumulative_requests").
		Script(elasticv7.NewScript(painlessScriptSource).Lang("painless").Params(scriptParams)).
		Window(10)
	r.logger.Info("Moving function aggregation built", zap.Any("scriptParams", scriptParams))

	return dateHistoAgg.
		SubAggregation("cumulative_requests", cumulativeSumAgg).
		SubAggregation(movFnAggName, movingFnAgg)
}

// executeSearch performs the Elasticsearch search.
func (r MetricsReader) executeSearch(ctx context.Context, p MetricsQueryParams) (*metrics.MetricFamily, error) {
	labels := []*metrics.Label{{Name: "service_name", Value: "driver"}}
	if p.GroupByOperation {
		p.metricName = strings.Replace(p.metricName, "service", "service_operation", 1)
		p.metricDesc += " & operation"
		labels = append(labels, &metrics.Label{Name: "operation", Value: "/FindNearest"})
	}

	searchResult, err := r.client.Search(searchIndex).
		Query(p.boolQuery).
		Size(0).
		Aggregation(dateHistAggName, p.dateHistoAgg).
		Do(ctx)
	if err != nil {
		return nil, err
	}

	rawJSON, err := json.MarshalIndent(searchResult, "", "  ")
	if err != nil {
		r.logger.Error("Failed to marshal search result to JSON", zap.Error(err))
		return nil, fmt.Errorf("failed to marshal search result to JSON: %w", err)
	}
	r.logger.Info("Elasticsearch search result", zap.String("result", string(rawJSON)))

	return r.metricsTranslator.ToMetricsFamily(
		p.metricName,
		p.metricDesc,
		labels,
		searchResult,
	)
}

// normalizeSpanKinds normalizes a slice of span kinds.
func normalizeSpanKinds(spanKinds []string) []string {
	if len(spanKinds) == 0 {
		return []string{"server"}
	}
	normalized := make([]string, len(spanKinds))
	for i, kind := range spanKinds {
		normalized[i] = normalizeSpanKind(kind)
	}
	return normalized
}

// normalizeSpanKind normalizes a single span kind.
func normalizeSpanKind(kind string) string {
	if strings.HasPrefix(kind, "SPAN_KIND_") {
		return strings.ToLower(strings.TrimPrefix(kind, "SPAN_KIND_"))
	}
	return strings.ToLower(kind)
}

// buildInterfaceSlice converts []string to []interface{} for elastic terms query.
func buildInterfaceSlice(s []string) []interface{} {
	ifaceSlice := make([]interface{}, len(s))
	for i, v := range s {
		ifaceSlice[i] = v
	}
	return ifaceSlice
}

// calculateTimeRange computes the time range for the query.
func (r MetricsReader) calculateTimeRange(params *metricstore.BaseQueryParameters) (TimeRange, error) {
	endTime := *params.EndTime
	startTime := endTime.Add(-*params.Lookback)
	extendedStartTime := startTime.Add(-5 * time.Minute)

	return TimeRange{
		startTimeMillis:         startTime.UnixMilli(),
		endTimeMillis:           endTime.UnixMilli(),
		extendedStartTimeMillis: extendedStartTime.UnixMilli(),
	}, nil
}
