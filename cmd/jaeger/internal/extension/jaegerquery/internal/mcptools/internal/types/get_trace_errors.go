// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetTraceErrorsInput defines the input parameters for the get_trace_errors MCP tool.
type GetTraceErrorsInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`

	// Offset is the zero-based starting index for error span pagination (optional).
	Offset int `json:"offset,omitempty" jsonschema:"Zero-based index offset for error span pagination (default: 0)"`

	// Limit is the maximum number of error span details to return (optional).
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of error span details to return (default: server limit)"`
}

// GetTraceErrorsOutput defines the output of the get_trace_errors MCP tool.
type GetTraceErrorsOutput struct {
	TraceID         string       `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	TotalErrorCount int          `json:"total_error_count" jsonschema:"Total number of error spans in the trace (may exceed the size of the spans list due to per-request limits)"`
	Offset          int          `json:"offset,omitempty" jsonschema:"Zero-based starting index offset for the returned error spans"`
	Spans           []SpanDetail `json:"spans,omitempty" jsonschema:"Error span details (possibly truncated to server-configured limit)"`
}
