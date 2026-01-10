// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// SearchTracesInput defines the input parameters for the search_traces MCP tool.
type SearchTracesInput struct {
	// StartTimeMin is the start of time interval (required).
	// Supports RFC3339 or relative time (e.g., "-1h", "-30m").
	StartTimeMin string `json:"start_time_min" jsonschema:"Start of time interval (RFC3339 or relative like -1h)"`

	// StartTimeMax is the end of time interval (optional, defaults to "now").
	// Supports RFC3339 or relative time (e.g., "now", "-1m").
	StartTimeMax string `json:"start_time_max,omitempty" jsonschema:"End of time interval (RFC3339 or relative like now). Default: now"`

	// ServiceName filters by service name (required).
	ServiceName string `json:"service_name" jsonschema:"Filter by service name. Use get_services to discover valid names"`

	// OperationName filters by operation/span name (optional).
	OperationName string `json:"operation_name,omitempty" jsonschema:"Filter by operation/span name"`

	// Attributes contains key-value pairs to match against span/resource attributes (optional).
	// Example: {"http.status_code": "500", "user.id": "12345"}
	Attributes map[string]string `json:"attributes,omitempty" jsonschema:"Key-value pairs to match against span/resource attributes"`

	// WithErrors filters to only return traces containing error spans (optional).
	WithErrors bool `json:"with_errors,omitempty" jsonschema:"If true only return traces containing error spans"`

	// DurationMin is the minimum duration filter (optional, e.g., "2s", "100ms").
	DurationMin string `json:"duration_min,omitempty" jsonschema:"Minimum duration filter (e.g. 2s 100ms)"`

	// DurationMax is the maximum duration filter (optional).
	DurationMax string `json:"duration_max,omitempty" jsonschema:"Maximum duration filter (e.g. 10s 1m)"`

	// SearchDepth defines the maximum search depth. Depending on the backend storage implementation,
	// this may behave like an SQL LIMIT clause. However, some implementations might not support
	// precise limits, and a larger value generally results in more traces being returned.
	// Default: 10, max: 100.
	SearchDepth int `json:"search_depth,omitempty" jsonschema:"Maximum search depth (default: 10 max: 100)"`
}

// SearchTracesOutput defines the output of the search_traces MCP tool.
type SearchTracesOutput struct {
	Traces []TraceSummary `json:"traces" jsonschema:"List of trace summaries matching the search criteria"`
}

// TraceSummary contains lightweight metadata about a single trace.
type TraceSummary struct {
	TraceID       string `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	RootService   string `json:"root_service" jsonschema:"Service name of the root span"`
	RootOperation string `json:"root_operation" jsonschema:"Operation name of the root span"`
	StartTime     string `json:"start_time" jsonschema:"Trace start time in RFC3339 format"`
	DurationMs    int64  `json:"duration_ms" jsonschema:"Total trace duration in milliseconds"`
	SpanCount     int    `json:"span_count" jsonschema:"Total number of spans in the trace"`
	ServiceCount  int    `json:"service_count" jsonschema:"Number of unique services in the trace"`
	HasErrors     bool   `json:"has_errors" jsonschema:"Whether the trace contains any error spans"`
}
