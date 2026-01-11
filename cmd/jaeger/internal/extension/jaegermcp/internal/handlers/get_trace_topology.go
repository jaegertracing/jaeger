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
	var spans []types.SpanNode
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
	root, err := h.buildTree(spans, input.Depth)
	if err != nil {
		return nil, types.GetTraceTopologyOutput{}, err
	}

	output := types.GetTraceTopologyOutput{
		TraceID: input.TraceID,
		Root:    root,
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
func buildSpanNode(pos jptrace.SpanIterPos, span ptrace.Span) types.SpanNode {
	// Get service name from resource attributes
	serviceName := ""
	if svc, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
		serviceName = svc.Str()
	}

	// Calculate duration in microseconds
	durationUs := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Microseconds()

	// Get parent span ID
	parentSpanID := ""
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().String()
	}

	return types.SpanNode{
		SpanID:     span.SpanID().String(),
		ParentID:   parentSpanID,
		Service:    serviceName,
		Operation:  span.Name(),
		StartTime:  span.StartTimestamp().AsTime().Format(time.RFC3339Nano),
		DurationUs: durationUs,
		Status:     span.Status().Code().String(),
		Children:   []types.SpanNode{},
	}
}

// buildTree constructs a tree from a flat list of spans.
func (h *getTraceTopologyHandler) buildTree(spans []types.SpanNode, maxDepth int) (types.SpanNode, error) {
	// Create a map of span ID to span for quick lookup
	spanMap := make(map[string]types.SpanNode)
	for i := range spans {
		spanMap[spans[i].SpanID] = spans[i]
	}

	// Create a map of parent ID to children
	childrenMap := make(map[string][]string)
	var rootSpanID string

	for i := range spans {
		if spans[i].ParentID == "" {
			// This is a root span
			rootSpanID = spans[i].SpanID
		} else {
			childrenMap[spans[i].ParentID] = append(childrenMap[spans[i].ParentID], spans[i].SpanID)
		}
	}

	// If no root span found, return error
	if rootSpanID == "" {
		return types.SpanNode{}, errors.New("could not build trace tree: no root span found")
	}

	// Build the tree recursively starting from the root
	return h.buildNode(rootSpanID, spanMap, childrenMap, 1, maxDepth), nil
}

// buildNode recursively builds a SpanNode and its children.
func (h *getTraceTopologyHandler) buildNode(
	spanID string,
	spanMap map[string]types.SpanNode,
	childrenMap map[string][]string,
	currentDepth int,
	maxDepth int,
) types.SpanNode {
	span, ok := spanMap[spanID]
	if !ok {
		return types.SpanNode{}
	}

	node := span
	node.Children = []types.SpanNode{}

	// Check if we should include children based on maxDepth
	// maxDepth of 0 means no limit, otherwise check if we've reached the limit
	if maxDepth == 0 || currentDepth < maxDepth {
		// Add children if any
		if childIDs, hasChildren := childrenMap[spanID]; hasChildren {
			for _, childID := range childIDs {
				childNode := h.buildNode(childID, spanMap, childrenMap, currentDepth+1, maxDepth)
				if childNode.SpanID != "" {
					node.Children = append(node.Children, childNode)
				}
			}
		}
	}

	return node
}
