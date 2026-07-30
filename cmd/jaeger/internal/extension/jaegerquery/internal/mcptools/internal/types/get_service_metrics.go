// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetServiceMetricsInput defines the input parameters for the get_service_metrics tool.
type GetServiceMetricsInput struct {
	// Services to fetch metrics for.
	Services []string `json:"services" jsonschema:"One or more service names to fetch metrics for"`

	// MetricType selects which RED metric to fetch.
	MetricType string `json:"metric_type" jsonschema:"Metric to fetch. One of: latency, call_rate, error_rate"`

	// Quantile applies to latency only.
	Quantile float64 `json:"quantile,omitempty" jsonschema:"Latency quantile in (0, 1], e.g. 0.95 for P95. Only used when metric_type is latency. Default: 0.95"`

	// EndTime is the end of the query range.
	EndTime string `json:"end_time,omitempty" jsonschema:"End of the time range as RFC3339, e.g. 2026-07-28T10:00:00Z. Default: now"`

	// Lookback is the window ending at end_time.
	Lookback string `json:"lookback,omitempty" jsonschema:"Query window ending at end_time, as a Go duration, e.g. 1h, 30m. Default: 1h"`

	// Step is the interval between data points.
	Step string `json:"step,omitempty" jsonschema:"Interval between data points, as a Go duration. Default: 1m. Smaller steps return more points and cost more context"`

	// RatePer is the sliding window for rate calculations.
	RatePer string `json:"rate_per,omitempty" jsonschema:"Sliding window over which per-second rates are computed, as a Go duration. Default: 10m"`

	// GroupByOperation splits each service's series per operation.
	GroupByOperation bool `json:"group_by_operation,omitempty" jsonschema:"If true, return one series per service+operation instead of one per service. Default: false"`

	// SpanKinds limits which span kinds are aggregated.
	SpanKinds []string `json:"span_kinds,omitempty" jsonschema:"Span kinds to include: server, client, internal, producer, consumer, unspecified. Default: [server]"`
}

// GetServiceMetricsOutput defines the output of the get_service_metrics tool.
type GetServiceMetricsOutput struct {
	// MetricType echoes the requested metric type.
	MetricType string `json:"metric_type" jsonschema:"The metric type that was queried"`

	// Quantile echoes the applied latency quantile, when relevant.
	Quantile float64 `json:"quantile,omitempty" jsonschema:"Latency quantile applied (latency only)"`

	// MetricName is the backend's name for this metric family.
	MetricName string `json:"metric_name,omitempty" jsonschema:"Backend metric family name"`

	// Description is the backend's help text, which states the metric semantics and units.
	Description string `json:"description,omitempty" jsonschema:"Backend description of the metric, including units"`

	// Metrics holds one time series per service (or service+operation).
	Metrics []ServiceMetricSeries `json:"metrics" jsonschema:"One time series per service, or per service+operation when group_by_operation is true"`
}

// ServiceMetricSeries is one labeled time series.
type ServiceMetricSeries struct {
	ServiceName   string            `json:"service_name" jsonschema:"Service the series belongs to"`
	OperationName string            `json:"operation_name,omitempty" jsonschema:"Operation, set when group_by_operation is true"`
	SpanKind      string            `json:"span_kind,omitempty" jsonschema:"Span kind the series was aggregated over"`
	DataPoints    []MetricDataPoint `json:"data_points" jsonschema:"Chronological data points"`
}

// MetricDataPoint is a single point in a series. Value is null when the backend
// has no data for that timestamp, as opposed to a measured zero.
type MetricDataPoint struct {
	TimestampMs int64    `json:"timestamp_ms" jsonschema:"Unix timestamp in milliseconds"`
	Value       *float64 `json:"value" jsonschema:"Metric value; null means no data at this timestamp, which is different from a measured 0"`
}
