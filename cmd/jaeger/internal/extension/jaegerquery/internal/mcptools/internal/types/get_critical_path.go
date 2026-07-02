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
	TraceID                string                `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	TotalDurationUs        uint64                `json:"total_duration_us" jsonschema:"Total trace duration in microseconds"`
	CriticalPathDurationUs uint64                `json:"critical_path_duration_us" jsonschema:"Total duration of critical path in microseconds"`
	Segments               []CriticalPathSegment `json:"segments" jsonschema:"Ordered list of span segments on the critical path"`
}

// CriticalPathSegment represents a span segment on the critical path.
type CriticalPathSegment struct {
	SpanID        string `json:"span_id" jsonschema:"Unique identifier for the span"`
	Service       string `json:"service" jsonschema:"Service name from resource attributes"`
	SpanName      string `json:"span_name" jsonschema:"Span name"`
	SelfTimeUs    uint64 `json:"self_time_us" jsonschema:"Time spent in this segment in microseconds"`
	StartOffsetUs uint64 `json:"start_offset_us" jsonschema:"Start time of this segment relative to trace start in microseconds"`
	EndOffsetUs   uint64 `json:"end_offset_us" jsonschema:"End time of this segment relative to trace start in microseconds"`
}
