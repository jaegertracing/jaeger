// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// FindSpansInput defines the input parameters for the find_spans MCP tool.
type FindSpansInput struct {
	// SQL is a read-only SELECT in the dialect of RFC 0019, over spans. It carries the window,
	// the filter, the order and the limit; there is no other way to shape the search.
	SQL string `json:"sql" jsonschema:"A SELECT statement over spans, with a bound on start_time. Read the jaeger-sql/SKILL.md skill before writing one"`

	// MaxResults caps the number of spans returned (optional). The statement's own LIMIT, when
	// present, overrides it. The server's MaxSearchResults caps both regardless of what is
	// requested here.
	MaxResults uint32 `json:"max_results,omitempty" jsonschema:"Maximum number of spans to return; overridden by the statement's own LIMIT. The server caps both"`
}

// FindSpansOutput defines the output of the find_spans MCP tool.
type FindSpansOutput struct {
	Spans []SpanDetail `json:"spans,omitempty" jsonschema:"List of spans matching the search criteria"`
	Error string       `json:"error,omitempty" jsonschema:"Error message if partial results were returned"`
}
