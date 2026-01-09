// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetCriticalPathInput represents the input for the get_critical_path tool
type GetCriticalPathInput struct {
	TraceID string `json:"trace_id" jsonschema:"required" jsonschema_description:"The trace ID to get critical path for"`
}

// CriticalPathSpan represents a span section on the critical path
type CriticalPathSpan struct {
	SpanID         string `json:"span_id" jsonschema_description:"The span ID"`
	Service        string `json:"service" jsonschema_description:"The service name"`
	Operation      string `json:"operation" jsonschema_description:"The operation/span name"`
	SelfTimeMs     uint64 `json:"self_time_ms" jsonschema_description:"Time spent in this section in milliseconds"`
	SectionStartMs uint64 `json:"section_start_ms" jsonschema_description:"Start time of this section relative to trace start in milliseconds"`
	SectionEndMs   uint64 `json:"section_end_ms" jsonschema_description:"End time of this section relative to trace start in milliseconds"`
}

// GetCriticalPathOutput represents the output of the get_critical_path tool
type GetCriticalPathOutput struct {
	TraceID                string             `json:"trace_id" jsonschema_description:"The trace ID"`
	TotalDurationMs        uint64             `json:"total_duration_ms" jsonschema_description:"Total trace duration in milliseconds"`
	CriticalPathDurationMs uint64             `json:"critical_path_duration_ms" jsonschema_description:"Total duration of critical path in milliseconds"`
	Path                   []CriticalPathSpan `json:"path" jsonschema_description:"Ordered list of spans on the critical path"`
}
