// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	openmetrics "github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

// metricsReaderInterface defines the subset of metricstore.Reader used by this handler.
// Keeping it as a local interface makes unit-testing straightforward.
type metricsReaderInterface interface {
	GetLatencies(ctx context.Context, params *metricstore.LatenciesQueryParameters) (*openmetrics.MetricFamily, error)
	GetCallRates(ctx context.Context, params *metricstore.CallRateQueryParameters) (*openmetrics.MetricFamily, error)
	GetErrorRates(ctx context.Context, params *metricstore.ErrorRateQueryParameters) (*openmetrics.MetricFamily, error)
}

type getServiceMetricsHandler struct {
	reader metricsReaderInterface
}

// NewGetServiceMetricsHandler creates the get_service_metrics MCP tool handler.
func NewGetServiceMetricsHandler(
	reader metricstore.Reader,
) mcp.ToolHandlerFor[types.GetServiceMetricsInput, types.GetServiceMetricsOutput] {
	h := &getServiceMetricsHandler{reader: reader}
	return h.handle
}

func (h *getServiceMetricsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetServiceMetricsInput,
) (*mcp.CallToolResult, types.GetServiceMetricsOutput, error) {
	// Validate required fields.
	if len(input.Services) == 0 {
		return nil, types.GetServiceMetricsOutput{}, errors.New("services must not be empty")
	}
	if input.MetricType == "" {
		return nil, types.GetServiceMetricsOutput{}, errors.New("metric_type must be one of: latency, call_rate, error_rate")
	}

	// Parse time parameters.
	endTime, err := parseTimeParam(input.EndTime)
	if err != nil || input.EndTime == "" {
		endTime = time.Now()
	}

	lookback := parseDurationOrDefault(input.Lookback, time.Hour)
	step := parseDurationOrDefault(input.Step, time.Minute)
	ratePer := parseDurationOrDefault(input.RatePer, time.Minute)

	quantile := input.Quantile
	if quantile <= 0 || quantile > 1 {
		quantile = 0.95
	}

	// Validate span kinds before passing through.
	if err := validateSpanKinds(input.SpanKinds); err != nil {
		return nil, types.GetServiceMetricsOutput{}, err
	}

	base := metricstore.BaseQueryParameters{
		ServiceNames:     input.Services,
		GroupByOperation: input.GroupByOperation,
		EndTime:          &endTime,
		Lookback:         &lookback,
		Step:             &step,
		RatePer:          &ratePer,
		SpanKinds:        input.SpanKinds,
	}

	var family *openmetrics.MetricFamily

	switch input.MetricType {
	case "latency":
		family, err = h.reader.GetLatencies(ctx, &metricstore.LatenciesQueryParameters{
			BaseQueryParameters: base,
			Quantile:            quantile,
		})
	case "call_rate":
		family, err = h.reader.GetCallRates(ctx, &metricstore.CallRateQueryParameters{
			BaseQueryParameters: base,
		})
	case "error_rate":
		family, err = h.reader.GetErrorRates(ctx, &metricstore.ErrorRateQueryParameters{
			BaseQueryParameters: base,
		})
	default:
		return nil, types.GetServiceMetricsOutput{}, fmt.Errorf(
			"unknown metric_type %q: must be one of latency, call_rate, error_rate",
			input.MetricType,
		)
	}

	if err != nil {
		if errors.Is(err, disabled.ErrDisabled) {
			return nil, types.GetServiceMetricsOutput{},
				errors.New("metrics storage is not configured; set a metrics backend in the jaegerquery config")
		}
		return nil, types.GetServiceMetricsOutput{}, fmt.Errorf("metrics query failed: %w", err)
	}

	output := convertMetricFamily(family, input.MetricType)
	return nil, output, nil
}

// convertMetricFamily converts the proto MetricFamily into the MCP output type.
func convertMetricFamily(family *openmetrics.MetricFamily, metricType string) types.GetServiceMetricsOutput {
	if family == nil {
		return types.GetServiceMetricsOutput{MetricType: metricType}
	}

	var serviceMetrics []types.ServiceMetric

	for _, m := range family.Metrics {
		if m == nil {
			continue
		}

		// Extract labels.
		var serviceName, operationName, spanKind string
		for _, lp := range m.Labels {
			switch lp.GetName() {
			case "service_name":
				serviceName = lp.GetValue()
			case "operation":
				operationName = lp.GetValue()
			case "span_kind":
				spanKind = lp.GetValue()
			}
		}

		// Convert data points.
		var dataPoints []types.MetricDataPoint
		for _, mp := range m.MetricPoints {
			if mp == nil {
				continue
			}

			// Convert gogo protobuf Timestamp to Unix milliseconds manually.
			var tsMs int64
			if mp.Timestamp != nil {
				tsMs = mp.Timestamp.Seconds*1000 + int64(mp.Timestamp.Nanos)/1_000_000
			}

			// GaugeValue has its own GetDoubleValue() method.
			var val float64
			if g := mp.GetGaugeValue(); g != nil {
				val = g.GetDoubleValue()
			}

			dataPoints = append(dataPoints, types.MetricDataPoint{
				TimestampMs: tsMs,
				Value:       val,
			})
		}

		serviceMetrics = append(serviceMetrics, types.ServiceMetric{
			ServiceName:   serviceName,
			OperationName: operationName,
			SpanKind:      spanKind,
			DataPoints:    dataPoints,
		})
	}

	return types.GetServiceMetricsOutput{
		MetricType: metricType,
		Metrics:    serviceMetrics,
	}
}

// parseDurationOrDefault parses a Go duration string, returning the default on failure or empty input.
func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// validateSpanKinds checks that all provided span kind strings are valid.
func validateSpanKinds(kinds []string) error {
	valid := map[string]bool{
		"SERVER":   true,
		"CLIENT":   true,
		"PRODUCER": true,
		"CONSUMER": true,
		"INTERNAL": true,
	}
	for _, k := range kinds {
		if !valid[k] {
			return fmt.Errorf("unknown span kind %q; valid values: SERVER, CLIENT, PRODUCER, CONSUMER, INTERNAL", k)
		}
	}
	return nil
}
