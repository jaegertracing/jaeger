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
	TraceID         string       `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	TotalErrorCount int          `json:"total_error_count" jsonschema:"Total number of error spans in the trace (may exceed the size of the spans list due to per-request limits)"`
	Spans           []SpanDetail `json:"spans,omitempty" jsonschema:"Error span details (possibly truncated to server-configured limit)"`
}
