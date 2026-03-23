// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
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

// rawSpan holds the raw data for a single span before path computation.
type rawSpan struct {
	spanID     string
	parentID   string // empty string if this is a root span
	service    string
	spanName   string
	startTime  string
	durationUs int64
	status     string
	startNano  int64 // used for sorting children by start time
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
	var spans []rawSpan
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceTopologyOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		traceFound = true

		// Iterate through all spans in the trace and collect them
		for pos, span := range jptrace.SpanIter(trace) {
			spans = append(spans, extractRawSpan(pos, span))
		}
	}

	if !traceFound {
		return nil, types.GetTraceTopologyOutput{}, errors.New("trace not found")
	}

	// Build the flat topology list from the collected spans
	output := types.GetTraceTopologyOutput{
		TraceID: input.TraceID,
		Spans:   h.buildFlatTopology(spans, input.Depth),
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

// extractRawSpan extracts minimal span information needed for topology.
func extractRawSpan(pos jptrace.SpanIterPos, span ptrace.Span) rawSpan {
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

	return rawSpan{
		spanID:     span.SpanID().String(),
		parentID:   parentSpanID,
		service:    serviceName,
		spanName:   span.Name(),
		startTime:  span.StartTimestamp().AsTime().Format(time.RFC3339Nano),
		durationUs: duration.Microseconds(),
		status:     span.Status().Code().String(),
		startNano:  span.StartTimestamp().AsTime().UnixNano(),
	}
}

// buildFlatTopology converts a flat slice of rawSpans into a depth-first ordered
// slice of TopologySpan where each span's Path encodes its ancestry as a
// slash-delimited sequence of span IDs from the root down to that span.
// Orphan spans (whose parent is absent from the trace) have their missing parent ID
// prepended to the path so the caller can identify the attachment point.
// When maxDepth > 0, spans beyond that depth are omitted and the last included
// ancestor records the count of excluded direct children in TruncatedChildren.
func (h *getTraceTopologyHandler) buildFlatTopology(spans []rawSpan, maxDepth int) []types.TopologySpan {
	// Create a map of span ID to span pointer for quick lookup
	byID := make(map[string]*rawSpan, len(spans))
	for i := range spans {
		byID[spans[i].spanID] = &spans[i]
	}

	// Build parent-child relationships; collect roots (parent absent from trace)
	childrenOf := make(map[string][]*rawSpan)
	var roots []*rawSpan
	for i := range spans {
		s := &spans[i]
		if s.parentID != "" && byID[s.parentID] != nil {
			childrenOf[s.parentID] = append(childrenOf[s.parentID], s)
		} else {
			roots = append(roots, s)
		}
	}

	// Sort roots and children by start time for a deterministic, meaningful order
	sortByStartNano(roots)
	for k := range childrenOf {
		sortByStartNano(childrenOf[k])
	}

	// DFS from each root to produce the flat list
	result := make([]types.TopologySpan, 0, len(spans))
	for _, root := range roots {
		// For orphans (has a parentID but parent not in trace), prepend the missing
		// parent ID to the path so the caller can identify the attachment point.
		var rootPath string
		if root.parentID != "" {
			rootPath = root.parentID + "/" + root.spanID
		} else {
			rootPath = root.spanID
		}
		h.dfs(root, rootPath, 1, maxDepth, childrenOf, &result)
	}
	return result
}

// dfs appends the current span to result and then recurses into its children.
// When maxDepth > 0 and the current span is at the depth limit, its children
// are counted but not visited, and TruncatedChildren is set on the emitted span.
func (h *getTraceTopologyHandler) dfs(
	span *rawSpan,
	path string,
	depth int,
	maxDepth int,
	childrenOf map[string][]*rawSpan,
	result *[]types.TopologySpan,
) {
	if maxDepth > 0 && depth > maxDepth {
		return
	}

	// Count and remove children beyond maxDepth
	truncated := 0
	if maxDepth > 0 && depth >= maxDepth {
		truncated = len(childrenOf[span.spanID])
	}

	*result = append(*result, types.TopologySpan{
		Path:              path,
		Service:           span.service,
		SpanName:          span.spanName,
		StartTime:         span.startTime,
		DurationUs:        span.durationUs,
		Status:            span.status,
		TruncatedChildren: truncated,
	})

	// Recursively process children if above the depth limit
	if truncated == 0 {
		for _, child := range childrenOf[span.spanID] {
			h.dfs(child, path+"/"+child.spanID, depth+1, maxDepth, childrenOf, result)
		}
	}
}

// sortByStartNano sorts a slice of rawSpan pointers by ascending start timestamp.
// Spans with equal timestamps are further ordered by span ID to make the sort
// deterministic regardless of the original collection order.
func sortByStartNano(spans []*rawSpan) {
	slices.SortStableFunc(spans, func(a, b *rawSpan) int {
		if a.startNano != b.startNano {
			return cmp.Compare(a.startNano, b.startNano)
		}
		return cmp.Compare(a.spanID, b.spanID)
	})
}
