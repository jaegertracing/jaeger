// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetCriticalPathInput defines the input parameters for the get_critical_path MCP tool.
type GetCriticalPathInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	// MaxSegments is the maximum number of segments to return in the response.
	// If not set, the server-configured default is used. The critical path is
	// computed over the full trace, and only the top-N segments with the
	// largest self_time_us are returned.
	MaxSegments *int `json:"max_segments,omitempty" jsonschema:"Maximum number of segments to return (default: server-configured limit)"`
}

// GetCriticalPathOutput defines the output of the get_critical_path MCP tool.
type GetCriticalPathOutput struct {
	TraceID                string                `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	TotalDurationUs        uint64                `json:"total_duration_us" jsonschema:"Total trace duration in microseconds"`
	CriticalPathDurationUs uint64                `json:"critical_path_duration_us" jsonschema:"Total duration of critical path in microseconds"`
	Segments               []CriticalPathSegment `json:"segments" jsonschema:"Ordered list of span segments on the critical path (may be truncated; compare total_segment_count with the size of segments to detect truncation)"`
	TotalSegmentCount      int                   `json:"total_segment_count" jsonschema:"Total number of segments in the critical path (may exceed the size of the segments list due to per-request limits)"`
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