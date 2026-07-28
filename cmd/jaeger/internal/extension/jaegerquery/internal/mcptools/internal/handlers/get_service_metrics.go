// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

const (
	metricTypeLatency   = "latency"
	metricTypeCallRate  = "call_rate"
	metricTypeErrorRate = "error_rate"

	defaultQuantile = 0.95
	defaultLookback = time.Hour
	// The step default is coarser than the HTTP API's 5s: at 5s a 1h window is
	// ~720 points per series, which is costly as model context (see the format
	// benchmark on issue #8409). 1m keeps a 1h window at 60 points.
	defaultStep    = time.Minute
	defaultRatePer = 10 * time.Minute

	// maxDataPointsPerSeries bounds lookback/step so a single call cannot
	// request an unbounded number of points per series.
	maxDataPointsPerSeries = 1500
)

// spanKindNames maps the accepted lowercase span kind names to the
// SpanKind enum names the metricstore expects.
var spanKindNames = map[string]string{
	"unspecified": metrics.SpanKind_SPAN_KIND_UNSPECIFIED.String(),
	"internal":    metrics.SpanKind_SPAN_KIND_INTERNAL.String(),
	"server":      metrics.SpanKind_SPAN_KIND_SERVER.String(),
	"client":      metrics.SpanKind_SPAN_KIND_CLIENT.String(),
	"producer":    metrics.SpanKind_SPAN_KIND_PRODUCER.String(),
	"consumer":    metrics.SpanKind_SPAN_KIND_CONSUMER.String(),
}

// getServiceMetricsHandler implements the get_service_metrics MCP tool. It
// exposes the RED metrics (latency, call rate, error rate) that Service
// Performance Monitoring already computes, as per-bucket time series.
type getServiceMetricsHandler struct {
	metricsReader metricstore.Reader
}

// NewGetServiceMetricsHandler creates a new get_service_metrics handler and
// returns the handler function.
func NewGetServiceMetricsHandler(
	metricsReader metricstore.Reader,
) mcp.ToolHandlerFor[types.GetServiceMetricsInput, types.GetServiceMetricsOutput] {
	h := &getServiceMetricsHandler{
		metricsReader: metricsReader,
	}
	return h.handle
}

// handle processes the get_service_metrics tool request.
func (h *getServiceMetricsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetServiceMetricsInput,
) (*mcp.CallToolResult, types.GetServiceMetricsOutput, error) {
	params, quantile, err := buildQueryParams(input)
	if err != nil {
		return nil, types.GetServiceMetricsOutput{}, err
	}

	var family *metrics.MetricFamily
	switch input.MetricType {
	case metricTypeLatency:
		family, err = h.metricsReader.GetLatencies(ctx, &metricstore.LatenciesQueryParameters{
			BaseQueryParameters: params,
			Quantile:            quantile,
		})
	case metricTypeCallRate:
		family, err = h.metricsReader.GetCallRates(ctx, &metricstore.CallRateQueryParameters{
			BaseQueryParameters: params,
		})
	case metricTypeErrorRate:
		family, err = h.metricsReader.GetErrorRates(ctx, &metricstore.ErrorRateQueryParameters{
			BaseQueryParameters: params,
		})
	default:
		return nil, types.GetServiceMetricsOutput{}, fmt.Errorf(
			"invalid metric_type %q: must be one of %s, %s, %s",
			input.MetricType, metricTypeLatency, metricTypeCallRate, metricTypeErrorRate,
		)
	}
	if err != nil {
		return nil, types.GetServiceMetricsOutput{}, fmt.Errorf("failed to get %s metrics: %w", input.MetricType, err)
	}

	output := types.GetServiceMetricsOutput{
		MetricType: input.MetricType,
		Metrics:    buildSeries(family),
	}
	if input.MetricType == metricTypeLatency {
		output.Quantile = quantile
	}
	if family != nil {
		output.MetricName = family.Name
		output.Description = family.Help
	}
	return nil, output, nil
}

// buildQueryParams validates the input and assembles the metricstore query
// parameters, returning the effective latency quantile alongside them.
func buildQueryParams(input types.GetServiceMetricsInput) (metricstore.BaseQueryParameters, float64, error) {
	var zero metricstore.BaseQueryParameters
	if len(input.Services) == 0 {
		return zero, 0, errors.New("at least one service is required")
	}

	quantile := input.Quantile
	if quantile == 0 {
		quantile = defaultQuantile
	}
	if quantile < 0 || quantile > 1 {
		return zero, 0, fmt.Errorf("invalid quantile %v: must be in (0, 1]", input.Quantile)
	}

	endTime := time.Now()
	if input.EndTime != "" {
		t, err := time.Parse(time.RFC3339, input.EndTime)
		if err != nil {
			return zero, 0, fmt.Errorf("invalid end_time: %w", err)
		}
		endTime = t
	}

	lookback, err := parsePositiveDuration("lookback", input.Lookback, defaultLookback)
	if err != nil {
		return zero, 0, err
	}
	step, err := parsePositiveDuration("step", input.Step, defaultStep)
	if err != nil {
		return zero, 0, err
	}
	ratePer, err := parsePositiveDuration("rate_per", input.RatePer, defaultRatePer)
	if err != nil {
		return zero, 0, err
	}
	if points := int64(lookback / step); points > maxDataPointsPerSeries {
		return zero, 0, fmt.Errorf(
			"lookback/step yields %d data points per series, exceeding the limit of %d: use a coarser step or a shorter lookback",
			points, maxDataPointsPerSeries,
		)
	}

	spanKinds, err := normalizeSpanKinds(input.SpanKinds)
	if err != nil {
		return zero, 0, err
	}

	return metricstore.BaseQueryParameters{
		ServiceNames:     input.Services,
		GroupByOperation: input.GroupByOperation,
		EndTime:          &endTime,
		Lookback:         &lookback,
		Step:             &step,
		RatePer:          &ratePer,
		SpanKinds:        spanKinds,
	}, quantile, nil
}

// parsePositiveDuration parses value as a Go duration, applying def when value
// is empty and rejecting non-positive results.
func parsePositiveDuration(name, value string, def time.Duration) (time.Duration, error) {
	if value == "" {
		return def, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s %q: must be positive", name, value)
	}
	return d, nil
}

// normalizeSpanKinds maps the accepted lowercase span kind names to the enum
// names the metricstore expects, defaulting to server, matching the HTTP API.
func normalizeSpanKinds(kinds []string) ([]string, error) {
	if len(kinds) == 0 {
		return []string{metrics.SpanKind_SPAN_KIND_SERVER.String()}, nil
	}
	normalized := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		mapped, ok := spanKindNames[strings.ToLower(kind)]
		if !ok {
			return nil, fmt.Errorf("invalid span kind %q: must be one of unspecified, internal, server, client, producer, consumer", kind)
		}
		normalized = append(normalized, mapped)
	}
	return normalized, nil
}

// buildSeries converts a MetricFamily into the tool's output series. A NaN
// gauge value, which the metricstore reports when the backend has no data for
// a timestamp, becomes a null value rather than a fake zero.
func buildSeries(family *metrics.MetricFamily) []types.ServiceMetricSeries {
	series := []types.ServiceMetricSeries{}
	if family == nil {
		return series
	}
	for _, metric := range family.Metrics {
		if metric == nil {
			continue
		}
		s := types.ServiceMetricSeries{
			DataPoints: []types.MetricDataPoint{},
		}
		for _, label := range metric.GetLabels() {
			switch label.Name {
			case "service_name":
				s.ServiceName = label.Value
			case "operation":
				s.OperationName = label.Value
			case "span_kind":
				s.SpanKind = label.Value
			default:
				// other labels are not part of the tool's output
			}
		}
		for _, point := range metric.GetMetricPoints() {
			if point == nil {
				continue
			}
			dp := types.MetricDataPoint{}
			if ts := point.GetTimestamp(); ts != nil {
				dp.TimestampMs = ts.Seconds*1000 + int64(ts.Nanos)/1_000_000
			}
			if gauge := point.GetGaugeValue(); gauge != nil {
				if v := gauge.GetDoubleValue(); !math.IsNaN(v) {
					dp.Value = &v
				}
			}
			s.DataPoints = append(s.DataPoints, dp)
		}
		series = append(series, s)
	}
	return series
}
