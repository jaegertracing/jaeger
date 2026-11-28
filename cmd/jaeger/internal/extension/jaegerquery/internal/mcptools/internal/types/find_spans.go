// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// NoColumnList is the sentence that tells an agent not to project. It appears in the
// description and the schema of find_spans and find_traces. A struct tag is a string literal
// and cannot refer to this constant, so the schema tags spell the sentence out. A test of each
// tool sends a column list through the real query service and requires the schema to carry the
// sentence exactly when the list is refused, so when a result is shaped by a column list the
// sentence has to go.
const NoColumnList = "Only SELECT *; no column list."

// FindSpansInput defines the input parameters for the find_spans MCP tool. The statement is
// in the dialect of RFC 0019.
type FindSpansInput struct {
	SQL string `json:"sql" jsonschema:"A SELECT * FROM spans WHERE ... statement with both bounds on start_time, and LIMIT for the number of spans. Only SELECT *; no column list. Read the jaeger-sql/SKILL.md skill before writing one"`
}

// FindSpansOutput defines the output of the find_spans MCP tool.
type FindSpansOutput struct {
	Spans []SpanSummary `json:"spans,omitempty"`
	Error string        `json:"error,omitempty"`
}

// SpanSummary identifies a matching span and says what it is, without its attributes,
// events or links. get_span_details returns those for the spans an agent picks.
type SpanSummary struct {
	SpanID       string `json:"span_id"`
	TraceID      string `json:"trace_id"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
	Service      string `json:"service"`
	SpanName     string `json:"span_name"`
	StartTime    string `json:"start_time"`
	DurationUs   int64  `json:"duration_us"`
	Status       string `json:"status"`
}
