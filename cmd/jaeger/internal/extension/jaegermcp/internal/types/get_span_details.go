// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetSpanDetailsInput defines the input parameters for the get_span_details MCP tool.
type GetSpanDetailsInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`

	// SpanIDs is a list of span IDs to fetch details for (required).
	// When a trace has more spans than the server limit, pass all span IDs and
	// set Offset to the returned NextOffset to paginate through them.
	SpanIDs []string `json:"span_ids" jsonschema:"List of span IDs to fetch details for"`

	// Offset is the starting index into SpanIDs for this page (optional, default: 0).
	// On the first call omit this field or set it to 0. On subsequent calls set it
	// to the NextOffset value returned by the previous response.
	Offset int `json:"offset,omitempty" jsonschema:"Starting index into span_ids for pagination (default 0)"`
}

// GetSpanDetailsOutput defines the output of the get_span_details MCP tool.
type GetSpanDetailsOutput struct {
	TraceID string       `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	Spans   []SpanDetail `json:"spans,omitempty" jsonschema:"List of span details"`
	Error   string       `json:"error,omitempty" jsonschema:"Error message if some spans were not found"`

	// HasMore is true when the number of requested span IDs exceeded the server
	// limit and only a partial page was returned. Call again with NextOffset to
	// retrieve the next page.
	HasMore bool `json:"has_more" jsonschema:"True if more spans remain to be fetched"`

	// NextOffset is the Offset value to use in the next call when HasMore is true.
	NextOffset int `json:"next_offset,omitempty" jsonschema:"Offset to use for the next page"`
}

// SpanDetail contains full OTLP span data including attributes, events, and links.
type SpanDetail struct {
	SpanID       string         `json:"span_id" jsonschema:"Unique identifier for the span"`
	TraceID      string         `json:"trace_id" jsonschema:"Trace identifier this span belongs to"`
	ParentSpanID string         `json:"parent_span_id,omitempty" jsonschema:"Parent span identifier"`
	Service      string         `json:"service" jsonschema:"Service name from resource attributes"`
	SpanName     string         `json:"span_name" jsonschema:"Span name"`
	StartTime    string         `json:"start_time" jsonschema:"Span start time in RFC3339 format"`
	DurationUs   int64          `json:"duration_us" jsonschema:"Span duration in microseconds"`
	Status       SpanStatus     `json:"status" jsonschema:"Span status information"`
	Attributes   map[string]any `json:"attributes,omitempty" jsonschema:"Span attributes"`
	Events       []SpanEvent    `json:"events,omitempty" jsonschema:"Span events"`
	Links        []SpanLink     `json:"links,omitempty" jsonschema:"Span links"`
}

// SpanStatus represents the status of a span.
type SpanStatus struct {
	Code    string `json:"code" jsonschema:"Status code (Unset Ok Error)"`
	Message string `json:"message,omitempty" jsonschema:"Status message"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name       string         `json:"name" jsonschema:"Event name"`
	Timestamp  string         `json:"timestamp" jsonschema:"Event timestamp in RFC3339 format"`
	Attributes map[string]any `json:"attributes,omitempty" jsonschema:"Event attributes"`
}

// SpanLink represents a link to another span.
type SpanLink struct {
	TraceID    string         `json:"trace_id" jsonschema:"Linked trace ID"`
	SpanID     string         `json:"span_id" jsonschema:"Linked span ID"`
	Attributes map[string]any `json:"attributes,omitempty" jsonschema:"Link attributes"`
}
