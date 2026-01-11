// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetCriticalPathInput defines the input parameters for the get_critical_path MCP tool.
type GetCriticalPathInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`
}

// GetCriticalPathOutput defines the output of the get_critical_path MCP tool.
type GetCriticalPathOutput struct {
	TraceID                string             `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	TotalDurationMs        uint64             `json:"total_duration_ms" jsonschema:"Total trace duration in milliseconds"`
	CriticalPathDurationMs uint64             `json:"critical_path_duration_ms" jsonschema:"Total duration of critical path in milliseconds"`
	Path                   []CriticalPathSpan `json:"path" jsonschema:"Ordered list of spans on the critical path"`
}

// CriticalPathSpan represents a span section on the critical path.
type CriticalPathSpan struct {
	SpanID         string `json:"span_id" jsonschema:"Unique identifier for the span"`
	Service        string `json:"service" jsonschema:"Service name from resource attributes"`
	Operation      string `json:"operation" jsonschema:"Operation/span name"`
	SelfTimeMs     uint64 `json:"self_time_ms" jsonschema:"Time spent in this section in milliseconds"`
	SectionStartMs uint64 `json:"section_start_ms" jsonschema:"Start time of this section relative to trace start in milliseconds"`
	SectionEndMs   uint64 `json:"section_end_ms" jsonschema:"End time of this section relative to trace start in milliseconds"`
}
