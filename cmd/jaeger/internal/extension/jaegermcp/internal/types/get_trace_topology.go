// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetTraceTopologyInput defines the input parameters for the get_trace_topology MCP tool.
type GetTraceTopologyInput struct {
	// TraceID is the unique identifier for the trace (required).
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`

	// Depth is the maximum depth of the tree to return (optional, default: 0 for full tree).
	// A depth of 0 means return the full tree, 1 means only root spans, 2 means root + children, etc.
	Depth int `json:"depth,omitempty" jsonschema:"Maximum depth of the tree. 0 for full tree"`
}

// GetTraceTopologyOutput defines the output of the get_trace_topology MCP tool.
type GetTraceTopologyOutput struct {
	TraceID string `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	// RootSpan is the root span of the trace tree. May be nil if trace has no root.
	// Declared as 'any' instead of '*SpanNode' because the MCP SDK's JSON schema
	// generator detects a cycle in the SpanNode type (due to Children []*SpanNode),
	// even when fields are marked with jsonschema:"-". Using 'any' bypasses type
	// analysis entirely while maintaining runtime type safety.
	RootSpan any `json:"root_span,omitempty"`
	// Orphans contains spans whose parent span is missing from the trace.
	// Declared as 'any' instead of '[]*SpanNode' for the same reason as RootSpan.
	Orphans any `json:"orphans,omitempty"`
}

// SpanNode represents a node in the trace tree structure.
// It contains minimal span information without attributes or events to keep the response compact.
type SpanNode struct {
	SpanID            string      `json:"span_id" jsonschema:"Unique identifier for the span"`
	ParentID          string      `json:"parent_id,omitempty" jsonschema:"Parent span identifier"`
	Service           string      `json:"service" jsonschema:"Service name from resource attributes"`
	SpanName          string      `json:"span_name" jsonschema:"Span name"`
	StartTime         string      `json:"start_time" jsonschema:"Span start time in RFC3339 format"`
	DurationUs        int64       `json:"duration_us" jsonschema:"Span duration in microseconds"`
	Status            string      `json:"status" jsonschema:"Span status (Unset Ok Error)"`
	Children          []*SpanNode `json:"children,omitempty" jsonschema:"-"`
	TruncatedChildren int         `json:"truncated_children,omitempty" jsonschema:"Number of children excluded due to depth limit"`
}
