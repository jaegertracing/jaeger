// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetServiceMetricsInput defines the input parameters for the get_service_metrics MCP tool.
type GetServiceMetricsInput struct {
	// Services is one or more service names to query metrics for (required).
	Services []string `json:"services" jsonschema:"One or more service names to query (e.g. [\"frontend\", \"backend\"])"`

	// MetricType selects which RED metric to retrieve.
	// One of: "latency", "call_rate", "error_rate".
	MetricType string `json:"metric_type" jsonschema:"Metric type to retrieve. One of: latency, call_rate, error_rate"`

	// Quantile is the latency percentile to compute (e.g. 0.95 for P95).
	// Only used when metric_type is \"latency\". Defaults to 0.95.
	Quantile float64 `json:"quantile,omitempty" jsonschema:"Latency percentile (0-1). Only used for latency. Default: 0.95"`

	// EndTime is the end of the query time range (optional, defaults to now).
	// Supports RFC3339 or relative time (e.g. \"-1h\", \"now\").
	EndTime string `json:"end_time,omitempty" jsonschema:"End of time range (RFC3339 or relative like -1h). Default: now"`

	// Lookback is the duration of the query window (optional, defaults to \"1h\").
	// Supports Go duration strings like \"1h\", \"30m\".
	Lookback string `json:"lookback,omitempty" jsonschema:"Query window duration (Go duration string, e.g. 1h, 30m). Default: 1h"`

	// Step is the resolution of the time series (optional, defaults to \"1m\").
	// Supports Go duration strings like \"1m\", \"5m\".
	Step string `json:"step,omitempty" jsonschema:"Time series resolution (Go duration string, e.g. 1m, 5m). Default: 1m"`

	// RatePer is the duration over which call/error rates are computed (optional, defaults to \"1m\").
	// Supports Go duration strings like \"1m\", \"5m\".
	RatePer string `json:"rate_per,omitempty" jsonschema:"Rate computation window (Go duration string, e.g. 1m). Default: 1m"`

	// GroupByOperation controls whether metrics are broken down per operation (optional, defaults to false).
	GroupByOperation bool `json:"group_by_operation,omitempty" jsonschema:"If true, break down metrics per operation/span name. Default: false"`

	// SpanKinds restricts metrics to specific span kinds (optional).
	// Values: SERVER, CLIENT, PRODUCER, CONSUMER, INTERNAL.
	SpanKinds []string `json:"span_kinds,omitempty" jsonschema:"Span kinds to include (SERVER, CLIENT, PRODUCER, CONSUMER, INTERNAL). Default: all"`
}

// GetServiceMetricsOutput defines the output of the get_service_metrics MCP tool.
type GetServiceMetricsOutput struct {
	// MetricType echoes back the requested metric type.
	MetricType string `json:"metric_type" jsonschema:"The metric type that was queried"`

	// Metrics contains the time series data per service (and optionally per operation).
	Metrics []ServiceMetric `json:"metrics" jsonschema:"List of metric time series, one per service/operation combination"`
}

// ServiceMetric holds a single time series for a service (and optionally operation).
type ServiceMetric struct {
	// ServiceName is the name of the service this metric belongs to.
	ServiceName string `json:"service_name" jsonschema:"Service name"`

	// OperationName is the operation/span name (only set when group_by_operation is true).
	OperationName string `json:"operation_name,omitempty" jsonschema:"Operation name (only set when group_by_operation=true)"`

	// SpanKind is the span kind this metric applies to (e.g. SERVER, CLIENT).
	SpanKind string `json:"span_kind,omitempty" jsonschema:"Span kind"`

	// DataPoints contains the time series samples.
	DataPoints []MetricDataPoint `json:"data_points" jsonschema:"Time series data points"`
}

// MetricDataPoint is a single sample in a time series.
type MetricDataPoint struct {
	// TimestampMs is the Unix timestamp in milliseconds.
	TimestampMs int64 `json:"timestamp_ms" jsonschema:"Unix timestamp in milliseconds"`

	// Value is the numeric value at this timestamp.
	Value float64 `json:"value" jsonschema:"Metric value at this timestamp"`
}