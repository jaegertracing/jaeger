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
	TraceID                string                `json:"trace_id"`
	TotalDurationUs        uint64                `json:"total_duration_us"`
	CriticalPathDurationUs uint64                `json:"critical_path_duration_us"`
	Segments               []CriticalPathSegment `json:"segments"`
}

// CriticalPathSegment represents a span segment on the critical path.
type CriticalPathSegment struct {
	SpanID        string `json:"span_id"`
	Service       string `json:"service"`
	SpanName      string `json:"span_name"`
	SelfTimeUs    uint64 `json:"self_time_us"`
	StartOffsetUs uint64 `json:"start_offset_us"`
	EndOffsetUs   uint64 `json:"end_offset_us"`
}
