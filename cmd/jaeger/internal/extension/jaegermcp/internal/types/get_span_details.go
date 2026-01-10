// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetSpanDetailsInput defines the input parameters for the get_span_details MCP tool.
type GetSpanDetailsInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`

	// SpanIDs is a list of span IDs to fetch details for (required).
	// It is recommended to limit this to 20 spans or fewer for optimal performance.
	SpanIDs []string `json:"span_ids" jsonschema:"List of span IDs to fetch details for. Recommended to limit to 20 spans or fewer"`
}

// GetSpanDetailsOutput defines the output of the get_span_details MCP tool.
type GetSpanDetailsOutput struct {
	TraceID string       `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	Spans   []SpanDetail `json:"spans" jsonschema:"List of span details"`
	Error   string       `json:"error,omitempty" jsonschema:"Error message if partial results were returned"`
}

// SpanDetail contains full OTLP span data including attributes, events, and links.
type SpanDetail struct {
	SpanID       string         `json:"span_id" jsonschema:"Unique identifier for the span"`
	TraceID      string         `json:"trace_id" jsonschema:"Trace identifier this span belongs to"`
	ParentSpanID string         `json:"parent_span_id,omitempty" jsonschema:"Parent span identifier"`
	Service      string         `json:"service" jsonschema:"Service name from resource attributes"`
	Operation    string         `json:"operation" jsonschema:"Operation/span name"`
	StartTime    string         `json:"start_time" jsonschema:"Span start time in RFC3339 format"`
	DurationMs   int64          `json:"duration_ms" jsonschema:"Span duration in milliseconds"`
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
