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
// Spans is a flat, depth-first ordered list. The Path field on each span encodes
// the tree structure as a slash-delimited sequence of span IDs from the root to
// that span (e.g. "rootID/parentID/spanID").
type GetTraceTopologyOutput struct {
	TraceID string         `json:"trace_id" jsonschema:"Unique identifier for the trace"`
	Spans   []TopologySpan `json:"spans"    jsonschema:"Flat depth-first list of spans; Path encodes parent-child relationships"`
}

// TopologySpan represents a span in the flat trace topology output.
// It contains minimal span information without attributes or events to keep the response compact.
// The Path field encodes the tree structure: it contains all span IDs from the root span down
// to this span, separated by slashes.
// Example: "rootSpanID/parentSpanID/thisSpanID"
// For orphan spans (whose parent is not present in the trace) the missing parent ID is prepended
// so the caller can identify the attachment point.
type TopologySpan struct {
	// Path is a slash-delimited sequence of span IDs from the root span to this span.
	Path              string `json:"path"                         jsonschema:"Slash-delimited span IDs from root to this span"`
	Service           string `json:"service"                      jsonschema:"Service name from resource attributes"`
	SpanName          string `json:"span_name"                    jsonschema:"Span name"`
	StartTime         string `json:"start_time"                   jsonschema:"Span start time in RFC3339 format"`
	DurationUs        int64  `json:"duration_us"                  jsonschema:"Span duration in microseconds"`
	Status            string `json:"status"                       jsonschema:"Span status (Unset Ok Error)"`
	TruncatedChildren int    `json:"truncated_children,omitempty" jsonschema:"Number of direct children excluded due to depth limit"`
}
