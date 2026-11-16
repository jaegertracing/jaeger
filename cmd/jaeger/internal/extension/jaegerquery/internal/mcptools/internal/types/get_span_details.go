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
	TraceID string       `json:"trace_id"`
	Spans   []SpanDetail `json:"spans,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// SpanDetail contains full OTLP span data including attributes, events, and links.
type SpanDetail struct {
	SpanID       string         `json:"span_id"`
	TraceID      string         `json:"trace_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Service      string         `json:"service"`
	SpanName     string         `json:"span_name"`
	StartTime    string         `json:"start_time"`
	DurationUs   int64          `json:"duration_us"`
	Status       SpanStatus     `json:"status"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Events       []SpanEvent    `json:"events,omitempty"`
	Links        []SpanLink     `json:"links,omitempty"`
}

// SpanStatus represents the status of a span.
type SpanStatus struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  string         `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanLink represents a link to another span.
type SpanLink struct {
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	Attributes map[string]any `json:"attributes,omitempty"`
}
