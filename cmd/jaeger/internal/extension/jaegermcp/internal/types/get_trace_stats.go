// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetTraceStatsInput defines the input parameters for the get_trace_stats MCP tool.
type GetTraceStatsInput struct {
	StartTimeMin string            `json:"start_time_min,omitempty"`
	StartTimeMax string            `json:"start_time_max,omitempty"`
	ServiceName  string            `json:"service_name"`
	SpanName     string            `json:"span_name,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	SearchDepth  int               `json:"search_depth,omitempty"`
}

// GetTraceStatsOutput defines the output of the get_trace_stats MCP tool.
type GetTraceStatsOutput struct {
	TraceCount    int            `json:"trace_count"`
	ErrorCount    int            `json:"error_count"`
	ErrorRate     float64        `json:"error_rate"`
	DurationStats DurationStats  `json:"duration_stats"`
	SpanStats     SpanStats      `json:"span_stats"`
	TopServices   []ServiceCount `json:"top_services"`
}

// DurationStats holds latency percentile statistics in microseconds.
type DurationStats struct {
	MinUs  int64 `json:"min_us"`
	MaxUs  int64 `json:"max_us"`
	MeanUs int64 `json:"mean_us"`
	P50Us  int64 `json:"p50_us"`
	P95Us  int64 `json:"p95_us"`
	P99Us  int64 `json:"p99_us"`
}

// SpanStats holds statistics about span counts per trace.
type SpanStats struct {
	MinSpans  int     `json:"min_spans"`
	MaxSpans  int     `json:"max_spans"`
	MeanSpans float64 `json:"mean_spans"`
}

// ServiceCount pairs a service name with the number of traces it participated in.
type ServiceCount struct {
	Service    string `json:"service"`
	TraceCount int    `json:"trace_count"`
}
