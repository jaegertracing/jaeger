// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetTraceErrorsInput defines the input parameters for the get_trace_errors MCP tool.
type GetTraceErrorsInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`
}

// GetTraceErrorsOutput defines the output of the get_trace_errors MCP tool.
type GetTraceErrorsOutput struct {
	TraceID         string      `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	TotalErrorCount int         `json:"total_error_count" jsonschema:"Total number of error spans in the trace (may exceed the size of the spans list due to per-request limits)"`
	Spans           []ErrorSpan `json:"spans,omitempty" jsonschema:"Error span listing (possibly truncated to server-configured limit). Use get_span_details for attributes, events, and links"`
}

// ErrorSpan is a compact listing of an error-status span. Full attributes,
// events, and links are available via get_span_details.
type ErrorSpan struct {
	SpanID        string `json:"span_id" jsonschema:"Unique identifier for the span"`
	ParentSpanID  string `json:"parent_span_id,omitempty" jsonschema:"Parent span identifier"`
	Service       string `json:"service" jsonschema:"Service name from resource attributes"`
	SpanName      string `json:"span_name" jsonschema:"Span name"`
	StatusMessage string `json:"status_message,omitempty" jsonschema:"Error status message"`
}
