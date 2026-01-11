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
	var spans []spanData
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceTopologyOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		traceFound = true

		// Iterate through all spans in the trace and collect them
		for pos, span := range jptrace.SpanIter(trace) {
			spanInfo := buildSpanData(pos, span)
			spans = append(spans, spanInfo)
		}
	}

	if !traceFound {
		return nil, types.GetTraceTopologyOutput{}, errors.New("trace not found")
	}

	// Build the tree structure from flat span list
	root := h.buildTree(spans, input.Depth)
	if root == nil {
		return nil, types.GetTraceTopologyOutput{}, errors.New("could not build trace tree: no root span found")
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

// spanData holds the minimal data needed to build the tree structure.
type spanData struct {
	spanID       string
	parentSpanID string
	service      string
	operation    string
	startTime    time.Time
	durationUs   int64
	status       string
}

// buildSpanData extracts minimal span information needed for topology.
func buildSpanData(pos jptrace.SpanIterPos, span ptrace.Span) spanData {
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

	return spanData{
		spanID:       span.SpanID().String(),
		parentSpanID: parentSpanID,
		service:      serviceName,
		operation:    span.Name(),
		startTime:    span.StartTimestamp().AsTime(),
		durationUs:   durationUs,
		status:       span.Status().Code().String(),
	}
}

// buildTree constructs a tree from a flat list of spans.
func (h *getTraceTopologyHandler) buildTree(spans []spanData, maxDepth int) *types.SpanNode {
	// Create a map of span ID to span data for quick lookup
	spanMap := make(map[string]spanData)
	for _, span := range spans {
		spanMap[span.spanID] = span
	}

	// Create a map of parent ID to children
	childrenMap := make(map[string][]string)
	var rootSpanID string

	for _, span := range spans {
		if span.parentSpanID == "" {
			// This is a root span
			rootSpanID = span.spanID
		} else {
			childrenMap[span.parentSpanID] = append(childrenMap[span.parentSpanID], span.spanID)
		}
	}

	// If no root span found, return nil
	if rootSpanID == "" {
		return nil
	}

	// Build the tree recursively starting from the root
	return h.buildNode(rootSpanID, spanMap, childrenMap, 1, maxDepth)
}

// buildNode recursively builds a SpanNode and its children.
func (h *getTraceTopologyHandler) buildNode(
	spanID string,
	spanMap map[string]spanData,
	childrenMap map[string][]string,
	currentDepth int,
	maxDepth int,
) *types.SpanNode {
	span, ok := spanMap[spanID]
	if !ok {
		return nil
	}

	node := &types.SpanNode{
		SpanID:     span.spanID,
		Service:    span.service,
		Operation:  span.operation,
		StartTime:  span.startTime.Format(time.RFC3339Nano),
		DurationUs: span.durationUs,
		Status:     span.status,
		Children:   []*types.SpanNode{},
	}

	// Check if we should include children based on maxDepth
	// maxDepth of 0 means no limit, otherwise check if we've reached the limit
	if maxDepth == 0 || currentDepth < maxDepth {
		// Add children if any
		if childIDs, hasChildren := childrenMap[spanID]; hasChildren {
			for _, childID := range childIDs {
				childNode := h.buildNode(childID, spanMap, childrenMap, currentDepth+1, maxDepth)
				if childNode != nil {
					node.Children = append(node.Children, childNode)
				}
			}
		}
	}

	return node
}
