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

// GetSpanDetailsInput defines the input parameters for the get_span_details MCP tool.
type GetSpanDetailsInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`

	// SpanIDs is a list of span IDs to fetch details for (required, max 20).
	SpanIDs []string `json:"span_ids" jsonschema:"List of span IDs to fetch details for (max 20)"`
}

// GetSpanDetailsOutput defines the output of the get_span_details MCP tool.
type GetSpanDetailsOutput struct {
	TraceID string       `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	Spans   []SpanDetail `json:"spans" jsonschema:"List of span details"`
}

// GetTraceErrorsInput defines the input parameters for the get_trace_errors MCP tool.
type GetTraceErrorsInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`
}

// GetTraceErrorsOutput defines the output of the get_trace_errors MCP tool.
type GetTraceErrorsOutput struct {
	TraceID    string       `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	ErrorCount int          `json:"error_count" jsonschema:"Number of spans with error status"`
	Spans      []SpanDetail `json:"spans" jsonschema:"List of error span details"`
}

// SpanDetail contains full OTLP span data including attributes, events, and links.
type SpanDetail struct {
	SpanID       string         `json:"span_id" jsonschema:"Unique identifier for the span"`
	TraceID      string         `json:"trace_id" jsonschema:"Trace identifier this span belongs to"`
	ParentSpanID string         `json:"parent_span_id,omitempty" jsonschema:"Parent span identifier"`
	Service      string         `json:"service" jsonschema:"Service name from resource attributes"`
	Operation    string         `json:"operation" jsonschema:"Operation/span name"`
	StartTime    string         `json:"start_time" jsonschema:"Span start time in RFC3339 format"`
	DurationMs   int64          `json:"duration_ms" jsonschema:"Span duration in milliseconds"`
	Status       SpanStatus     `json:"status" jsonschema:"Span status information"`
	Attributes   map[string]any `json:"attributes,omitempty" jsonschema:"Span attributes"`
	Events       []SpanEvent    `json:"events,omitempty" jsonschema:"Span events"`
	Links        []SpanLink     `json:"links,omitempty" jsonschema:"Span links"`
}

// SpanStatus represents the status of a span.
type SpanStatus struct {
	Code    string `json:"code" jsonschema:"Status code (UNSET OK ERROR)"`
	Message string `json:"message,omitempty" jsonschema:"Status message"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name       string         `json:"name" jsonschema:"Event name"`
	Timestamp  string         `json:"timestamp" jsonschema:"Event timestamp in RFC3339 format"`
	Attributes map[string]any `json:"attributes,omitempty" jsonschema:"Event attributes"`
}

// SpanLink represents a link to another span.
type SpanLink struct {
	TraceID    string         `json:"trace_id" jsonschema:"Linked trace ID"`
	SpanID     string         `json:"span_id" jsonschema:"Linked span ID"`
	Attributes map[string]any `json:"attributes,omitempty" jsonschema:"Link attributes"`
}
