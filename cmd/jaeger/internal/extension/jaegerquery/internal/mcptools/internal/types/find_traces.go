// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// FindTracesInput defines the input parameters for the find_traces MCP tool. The statement is
// in the dialect of RFC 0019 and carries the whole search: the filter, the time window and,
// through LIMIT, the number of traces.
type FindTracesInput struct {
	SQL string `json:"sql" jsonschema:"A SELECT DISTINCT trace_id FROM spans WHERE ... or SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...) statement with both bounds on start_time, and LIMIT for the number of traces. Only SELECT *; no column list. Read the jaeger-sql/SKILL.md skill before writing one"`
}

// FindTracesOutput defines the output of the find_traces MCP tool.
type FindTracesOutput struct {
	Traces []TraceSummary `json:"traces,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// TraceSummary contains lightweight metadata about a single trace.
type TraceSummary struct {
	TraceID      string   `json:"trace_id"`
	RootService  string   `json:"root_service"`
	RootSpanName string   `json:"root_span_name"`
	StartTime    string   `json:"start_time"`
	DurationUs   int64    `json:"duration_us"`
	SpanCount    int      `json:"span_count"`
	ServiceCount int      `json:"service_count"`
	Services     []string `json:"services"`
	HasErrors    bool     `json:"has_errors"`
}
