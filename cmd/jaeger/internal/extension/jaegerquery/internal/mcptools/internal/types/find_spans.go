// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// FindSpansInput defines the input parameters for the find_spans MCP tool.
type FindSpansInput struct {
	// SQL is a read-only SELECT in the dialect of RFC 0019, over spans. It carries the window,
	// the filter, the order and the limit; there is no other way to shape the search.
	SQL string `json:"sql" jsonschema:"A SELECT * FROM spans WHERE ... statement with both bounds on start_time, and LIMIT for the number of spans. Read the jaeger-sql/SKILL.md skill before writing one"`
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
