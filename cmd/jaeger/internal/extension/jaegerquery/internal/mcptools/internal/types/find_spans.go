// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// NoColumnList is the sentence that tells an agent not to project, in the description and
// the schema of find_spans and find_traces and in the refusal each returns for a column list.
// The schema tags below repeat it verbatim, since a struct tag cannot name a constant, and a
// test holds them to it. When a result is shaped by a column list, the refusal goes and this
// sentence goes with it.
const NoColumnList = "Only SELECT *; no column list."

// FindSpansInput defines the input parameters for the find_spans MCP tool. The statement is
// in the dialect of RFC 0019.
type FindSpansInput struct {
	SQL string `json:"sql" jsonschema:"A SELECT * FROM spans WHERE ... statement with both bounds on start_time, and LIMIT for the number of spans. Only SELECT *; no column list. Read the jaeger-sql/SKILL.md skill before writing one"`
}

// FindSpansOutput defines the output of the find_spans MCP tool.
type FindSpansOutput struct {
	Spans []SpanSummary `json:"spans,omitempty" jsonschema:"List of spans matching the search criteria"`
	Error string        `json:"error,omitempty" jsonschema:"Error message if partial results were returned"`
}

// SpanSummary identifies a matching span and says what it is, without its attributes,
// events or links. get_span_details returns those for the spans an agent picks.
type SpanSummary struct {
	SpanID       string `json:"span_id" jsonschema:"Unique identifier for the span"`
	TraceID      string `json:"trace_id" jsonschema:"Trace identifier this span belongs to"`
	ParentSpanID string `json:"parent_span_id,omitempty" jsonschema:"Parent span identifier"`
	Service      string `json:"service" jsonschema:"Service name from resource attributes"`
	SpanName     string `json:"span_name" jsonschema:"Span name"`
	StartTime    string `json:"start_time" jsonschema:"Span start time in RFC3339 format"`
	DurationUs   int64  `json:"duration_us" jsonschema:"Span duration in microseconds"`
	Status       string `json:"status" jsonschema:"Span status (Unset Ok Error)"`
}
