// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// FindSpansInput defines the input parameters for the find_spans MCP tool. Unlike
// search_traces, the time range is required (RFC 0016 §4.5): a span search has no other
// bound to fall back to.
type FindSpansInput struct {
	// StartTimeMin is the start of the time interval (required).
	// Supports RFC3339 or relative time (e.g., "-1h", "-30m").
	StartTimeMin string `json:"start_time_min" jsonschema:"Start of time interval (RFC3339 or relative like -1h). Required"`

	// StartTimeMax is the end of the time interval (required).
	// Supports RFC3339 or relative time (e.g., "now", "-1m").
	StartTimeMax string `json:"start_time_max" jsonschema:"End of time interval (RFC3339 or relative like now). Required"`

	// ServiceName filters by the span's resource service name (optional).
	ServiceName string `json:"service_name,omitempty" jsonschema:"Filter by service name. Use get_services to discover valid names"`

	// SpanName filters by span name (optional).
	SpanName string `json:"span_name,omitempty" jsonschema:"Filter by span name"`

	// Attributes contains key-value pairs to match against span/resource attributes (optional).
	// A value matches whatever kind the attribute is actually stored as (string, number or
	// bool), not only a literal string comparison.
	Attributes map[string]string `json:"attributes,omitempty" jsonschema:"Key-value pairs to match against span/resource attributes"`

	// WithErrors filters to only return error-status spans (optional).
	WithErrors bool `json:"with_errors,omitempty" jsonschema:"If true only return spans with an error status"`

	// DurationMin is the minimum span duration filter (optional, e.g., "2s", "100ms").
	DurationMin string `json:"duration_min,omitempty" jsonschema:"Minimum span duration filter (e.g. 2s 100ms)"`

	// DurationMax is the maximum span duration filter (optional).
	DurationMax string `json:"duration_max,omitempty" jsonschema:"Maximum span duration filter (e.g. 10s 1m)"`

	// MaxResults caps the number of spans returned (optional). Server configuration
	// (MaxSpanDetailsPerRequest) caps it further regardless of what is requested here.
	MaxResults int `json:"max_results,omitempty" jsonschema:"Maximum number of spans to return (default and max controlled by server config)"`
}

// FindSpansOutput defines the output of the find_spans MCP tool.
type FindSpansOutput struct {
	Spans []SpanDetail `json:"spans,omitempty" jsonschema:"List of spans matching the search criteria"`
	Error string       `json:"error,omitempty" jsonschema:"Error message if partial results were returned, or if the backend cannot serve this query"`
}
