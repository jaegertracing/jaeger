// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetSpanNamesInput defines the input parameters for the get_span_names MCP tool.
type GetSpanNamesInput struct {
	// ServiceName filters by service name (required).
	ServiceName string `json:"service_name" jsonschema:"Filter by service name (required). Use get_services to discover valid names"`

	// Pattern is an optional regex pattern to filter span names (optional).
	Pattern string `json:"pattern,omitempty" jsonschema:"Optional regex pattern to filter span names"`

	// SpanKind filters by span kind (optional, e.g., SERVER, CLIENT, PRODUCER, CONSUMER, INTERNAL).
	SpanKind string `json:"span_kind,omitempty" jsonschema:"Optional span kind filter (e.g. SERVER CLIENT PRODUCER CONSUMER INTERNAL)"`

	// Limit is the maximum number of span names to return (optional, default: 100).
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of span names to return (default: 100)"`
}

// GetSpanNamesOutput defines the output of the get_span_names MCP tool.
type GetSpanNamesOutput struct {
	SpanNames []SpanNameInfo `json:"span_names,omitempty" jsonschema:"List of span names for the service"`
}

// SpanNameInfo contains information about a span name.
type SpanNameInfo struct {
	// Name is the span name.
	Name string `json:"name" jsonschema:"Span name"`

	// SpanKind is the span kind (e.g., SERVER, CLIENT).
	SpanKind string `json:"span_kind,omitempty" jsonschema:"Span kind if available"`
}
