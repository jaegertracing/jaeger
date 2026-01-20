// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// getTraceTopologyHandler implements the get_trace_topology MCP tool.
// This tool returns the structural tree of a trace showing parent-child relationships,
// timing, and error locations WITHOUT returning attributes or logs to keep the response compact.
type getTraceTopologyHandler struct {
	queryService queryServiceGetTracesInterface
}

// NewGetTraceTopologyHandler creates a new get_trace_topology handler and returns the handler function.
func NewGetTraceTopologyHandler(
	queryService *querysvc.QueryService,
) mcp.ToolHandlerFor[types.GetTraceTopologyInput, types.GetTraceTopologyOutput] {
	h := &getTraceTopologyHandler{
		queryService: queryService,
	}
	return h.handle
}

// handle processes the get_trace_topology tool request.
func (h *getTraceTopologyHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetTraceTopologyInput,
) (*mcp.CallToolResult, types.GetTraceTopologyOutput, error) {
	// Build query parameters (includes validation)
	params, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetTraceTopologyOutput{}, err
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// Wrap with AggregateTraces to ensure each ptrace.Traces contains a complete trace
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Collect all spans from the trace
	var spans []*types.SpanNode
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceTopologyOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		traceFound = true

		// Iterate through all spans in the trace and collect them
		for pos, span := range jptrace.SpanIter(trace) {
			spanInfo := buildSpanNode(pos, span)
			spans = append(spans, spanInfo)
		}
	}

	if !traceFound {
		return nil, types.GetTraceTopologyOutput{}, errors.New("trace not found")
	}

	// Build the tree structure from flat span list
	rootSpan, orphans := h.buildTree(spans, input.Depth)

	output := types.GetTraceTopologyOutput{
		TraceID:  input.TraceID,
		RootSpan: rootSpan,
		Orphans:  orphans,
	}

	return nil, output, nil
}

// buildQuery converts GetTraceTopologyInput to querysvc.GetTraceParams.
func (*getTraceTopologyHandler) buildQuery(input types.GetTraceTopologyInput) (querysvc.GetTraceParams, error) {
	// Validate input
	if input.TraceID == "" {
		return querysvc.GetTraceParams{}, errors.New("trace_id is required")
	}

	traceID, err := parseTraceID(input.TraceID)
	if err != nil {
		return querysvc.GetTraceParams{}, fmt.Errorf("invalid trace_id: %w", err)
	}

	return querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{TraceID: traceID},
		},
		RawTraces: false, // We want adjusted traces
	}, nil
}

// buildSpanNode extracts minimal span information needed for topology.
func buildSpanNode(pos jptrace.SpanIterPos, span ptrace.Span) *types.SpanNode {
	// Get service name from resource attributes
	serviceName := ""
	if svc, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
		serviceName = svc.Str()
	}

	// Calculate duration
	duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())

	// Get parent span ID
	parentSpanID := ""
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().String()
	}

	return &types.SpanNode{
		SpanID:     span.SpanID().String(),
		ParentID:   parentSpanID,
		Service:    serviceName,
		Operation:  span.Name(),
		StartTime:  span.StartTimestamp().AsTime().Format(time.RFC3339Nano),
		DurationUs: duration.Microseconds(),
		Status:     span.Status().Code().String(),
		Children:   []*types.SpanNode{},
	}
}

// buildTree constructs a tree from a flat list of spans.
// It returns the root span and a list of orphaned spans (spans whose parent is missing).
func (h *getTraceTopologyHandler) buildTree(spans []*types.SpanNode, maxDepth int) (*types.SpanNode, []*types.SpanNode) {
	// Create a map of span ID to span pointer for quick lookup
	spanMap := make(map[string]*types.SpanNode)
	for i := range spans {
		spanMap[spans[i].SpanID] = spans[i]
	}

	var rootSpan *types.SpanNode
	var orphans []*types.SpanNode

	// Build parent-child relationships by connecting spans directly
	for i := range spans {
		if spans[i].ParentID == "" {
			// This is a root span
			if rootSpan == nil {
				rootSpan = spans[i]
			} else {
				// Multiple roots - treat additional roots as orphans
				orphans = append(orphans, spans[i])
			}
		} else {
			// Find parent and attach this span as a child
			parent, parentExists := spanMap[spans[i].ParentID]
			if parentExists {
				parent.Children = append(parent.Children, spans[i])
			} else {
				// Parent not found - this is an orphan
				orphans = append(orphans, spans[i])
			}
		}
	}

	// Apply depth limiting if needed
	if maxDepth > 0 && rootSpan != nil {
		h.limitDepth(rootSpan, 1, maxDepth)
	}
	for _, orphan := range orphans {
		if maxDepth > 0 {
			h.limitDepth(orphan, 1, maxDepth)
		}
	}

	return rootSpan, orphans
}

// limitDepth recursively limits the depth of the tree by removing children beyond maxDepth.
func (h *getTraceTopologyHandler) limitDepth(node *types.SpanNode, currentDepth int, maxDepth int) {
	if currentDepth >= maxDepth {
		// Count and remove all children at this depth
		node.TruncatedChildren = len(node.Children)
		node.Children = nil
		return
	}

	// Recursively limit depth for children
	for _, child := range node.Children {
		h.limitDepth(child, currentDepth+1, maxDepth)
	}
}
