// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetTraceStatsInput defines the input parameters for the get_trace_stats MCP tool.
// It accepts the same filters as search_traces but returns aggregate statistics
// instead of individual trace summaries.
type GetTraceStatsInput struct {
// StartTimeMin is the start of time interval (optional, defaults to "-1h").
// Supports RFC3339 or relative time (e.g., "-1h", "-30m").
StartTimeMin string `json:"start_time_min,omitempty" jsonschema:"Start of time interval (RFC3339 or relative like -1h). Default: -1h"`

// StartTimeMax is the end of time interval (optional, defaults to "now").
// Supports RFC3339 or relative time (e.g., "now", "-1m").
StartTimeMax string `json:"start_time_max,omitempty" jsonschema:"End of time interval (RFC3339 or relative like now). Default: now"`

// ServiceName filters by service name (required).
ServiceName string `json:"service_name" jsonschema:"Filter by service name. Use get_services to discover valid names"`

// SpanName filters by span name (optional).
SpanName string `json:"span_name,omitempty" jsonschema:"Filter by span name"`

// Attributes contains key-value pairs to match against span/resource attributes (optional).
Attributes map[string]string `json:"attributes,omitempty" jsonschema:"Key-value pairs to match against span/resource attributes"`

// WithErrors filters to only analyse traces containing error spans (optional).
WithErrors bool `json:"with_errors,omitempty" jsonschema:"If true only analyse traces containing error spans"`

// DurationMin is the minimum trace duration filter (optional, e.g., "2s", "100ms").
DurationMin string `json:"duration_min,omitempty" jsonschema:"Minimum duration filter (e.g. 2s 100ms)"`

// DurationMax is the maximum trace duration filter (optional).
DurationMax string `json:"duration_max,omitempty" jsonschema:"Maximum duration filter (e.g. 10s 1m)"`

// SearchDepth defines the maximum number of traces to analyse for statistics.
// Default: 100, maximum is controlled by server configuration (MaxSearchResults).
SearchDepth int `json:"search_depth,omitempty" jsonschema:"Maximum number of traces to analyse (default: 100, max controlled by server config)"`
}

// GetTraceStatsOutput defines the output of the get_trace_stats MCP tool.
type GetTraceStatsOutput struct {
// TraceCount is the total number of traces analysed.
TraceCount int `json:"trace_count" jsonschema:"Total number of traces analysed"`

// ErrorCount is the number of traces that contain at least one error span.
ErrorCount int `json:"error_count" jsonschema:"Number of traces containing at least one error span"`

// ErrorRate is the fraction of traces with errors, expressed as a value between 0 and 1.
ErrorRate float64 `json:"error_rate" jsonschema:"Fraction of traces with errors (0.0-1.0)"`

// DurationStats contains latency percentile statistics across all analysed traces (in microseconds).
DurationStats DurationStats `json:"duration_stats" jsonschema:"Latency statistics across all analysed traces"`

// SpanStats contains statistics about span counts across all analysed traces.
SpanStats SpanStats `json:"span_stats" jsonschema:"Span count statistics across all analysed traces"`

// TopServices lists the services that appear most frequently across the analysed traces,
// ordered by descending occurrence count.
TopServices []ServiceCount `json:"top_services" jsonschema:"Services ordered by descending occurrence count across traces"`
}

// DurationStats holds latency percentile statistics in microseconds.
type DurationStats struct {
MinUs  int64 `json:"min_us"  jsonschema:"Minimum trace duration in microseconds"`
MaxUs  int64 `json:"max_us"  jsonschema:"Maximum trace duration in microseconds"`
MeanUs int64 `json:"mean_us" jsonschema:"Mean trace duration in microseconds"`
P50Us  int64 `json:"p50_us"  jsonschema:"50th-percentile (median) trace duration in microseconds"`
P95Us  int64 `json:"p95_us"  jsonschema:"95th-percentile trace duration in microseconds"`
P99Us  int64 `json:"p99_us"  jsonschema:"99th-percentile trace duration in microseconds"`
}

// SpanStats holds statistics about span counts per trace.
type SpanStats struct {
MinSpans  int     `json:"min_spans"  jsonschema:"Minimum span count across all analysed traces"`
MaxSpans  int     `json:"max_spans"  jsonschema:"Maximum span count across all analysed traces"`
MeanSpans float64 `json:"mean_spans" jsonschema:"Mean span count across all analysed traces"`
}

// ServiceCount pairs a service name with the number of traces it participated in.
type ServiceCount struct {
Service    string `json:"service"     jsonschema:"Service name"`
TraceCount int    `json:"trace_count" jsonschema:"Number of analysed traces this service participated in"`
}