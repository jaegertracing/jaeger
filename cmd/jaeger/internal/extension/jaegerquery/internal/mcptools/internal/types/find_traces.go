// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// FindTracesInput defines the input parameters for the find_traces MCP tool. The statement is
// in the dialect of RFC 0019 and carries the whole search: the filter, the time window and,
// through LIMIT, the number of traces.
type FindTracesInput struct {
	SQL string `json:"sql" jsonschema:"A SELECT DISTINCT trace_id FROM spans WHERE ... or SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...) statement, with a column list over traces in place of *, with both bounds on start_time, and LIMIT for the number of traces. Read the jaeger-sql/SKILL.md skill before writing one"`
}

// FindTracesOutput defines the output of the find_traces MCP tool.
type FindTracesOutput struct {
	Traces []TraceSummary `json:"traces,omitempty" jsonschema:"List of trace summaries matching the search criteria"`
	Error  string         `json:"error,omitempty" jsonschema:"Error message if partial results were returned"`
}

// TraceSummary contains lightweight metadata about a single trace.
type TraceSummary struct {
	TraceID      string   `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	RootService  string   `json:"root_service" jsonschema:"Service name of the root span"`
	RootSpanName string   `json:"root_span_name" jsonschema:"Span name of the root span"`
	StartTime    string   `json:"start_time" jsonschema:"Trace start time in RFC3339 format"`
	DurationUs   int64    `json:"duration_us" jsonschema:"Total trace duration in microseconds"`
	SpanCount    int      `json:"span_count" jsonschema:"Total number of spans in the trace"`
	ServiceCount int      `json:"service_count" jsonschema:"Number of unique services in the trace"`
	Services     []string `json:"services" jsonschema:"Sorted list of unique service names participating in the trace"`
	HasErrors    bool     `json:"has_errors" jsonschema:"Whether the trace contains any error spans"`
}
