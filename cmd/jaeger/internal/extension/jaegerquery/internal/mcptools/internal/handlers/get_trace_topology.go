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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// getTraceTopologyHandler implements the get_trace_topology MCP tool.
// This tool returns the structural tree of a trace showing parent-child relationships,
// timing, and error locations WITHOUT returning attributes or logs to keep the response compact.
type getTraceTopologyHandler struct {
	queryService             queryServiceGetTracesInterface
	maxSpanDetailsPerRequest int
}

// NewGetTraceTopologyHandler creates a new get_trace_topology handler and returns the handler function.
func NewGetTraceTopologyHandler(
	queryService *querysvc.QueryService,
	maxSpanDetailsPerRequest int,
) mcp.ToolHandlerFor[types.GetTraceTopologyInput, types.GetTraceTopologyOutput] {
	h := &getTraceTopologyHandler{
		queryService:             queryService,
		maxSpanDetailsPerRequest: maxSpanDetailsPerRequest,
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

	// Aggregate the full trace so a count-limit subset can still tell
	// "parent dropped by the cap" from "parent never existed in the trace".
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Collect all spans from the trace
	var spans []rawSpan
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceTopologyOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		traceFound = true

		for pos, span := range jptrace.SpanIter(trace) {
			spans = append(spans, extractRawSpan(pos, span))
		}
	}

	if !traceFound {
		return nil, types.GetTraceTopologyOutput{}, errors.New("trace not found")
	}

	allIDs := make(map[string]struct{}, len(spans))
	byID := make(map[string]*rawSpan, len(spans))
	for i := range spans {
		allIDs[spans[i].spanID] = struct{}{}
		byID[spans[i].spanID] = &spans[i]
	}
	fullChildrenOf := make(map[string][]*rawSpan)
	for i := range spans {
		s := &spans[i]
		if s.parentID != "" && byID[s.parentID] != nil {
			fullChildrenOf[s.parentID] = append(fullChildrenOf[s.parentID], s)
		}
	}

	returned := spans
	if h.maxSpanDetailsPerRequest > 0 && len(spans) > h.maxSpanDetailsPerRequest {
		returned = spans[:h.maxSpanDetailsPerRequest]
	}

	output := types.GetTraceTopologyOutput{
		TraceID: input.TraceID,
		Spans:   h.buildFlatTopology(returned, input.Depth, allIDs, fullChildrenOf),
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
// Orphan spans (whose parent is absent from the stored trace) have their missing
// parent ID prepended to the path so the caller can identify the attachment point.
// Parents that existed in the trace but were omitted by the per-request count
// limit are not prefixed that way; TruncatedChildren on a surviving ancestor
// records how many direct children were dropped.
// When maxDepth > 0, spans beyond that depth are omitted and the last included
// ancestor records the count of excluded direct children in TruncatedChildren.
func (h *getTraceTopologyHandler) buildFlatTopology(
	spans []rawSpan,
	maxDepth int,
	allIDs map[string]struct{},
	fullChildrenOf map[string][]*rawSpan,
) []types.TopologySpan {
	byID := make(map[string]*rawSpan, len(spans))
	returnedIDs := make(map[string]struct{}, len(spans))
	for i := range spans {
		byID[spans[i].spanID] = &spans[i]
		returnedIDs[spans[i].spanID] = struct{}{}
	}

	// Build parent-child relationships among returned spans; collect forest roots
	// (true roots, genuine orphans, and spans whose parent was dropped by the cap).
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

	sortByStartNano(roots)
	for k := range childrenOf {
		sortByStartNano(childrenOf[k])
	}

	result := make([]types.TopologySpan, 0, len(spans))
	for _, root := range roots {
		var rootPath string
		if root.parentID != "" {
			if _, inTrace := allIDs[root.parentID]; inTrace {
				// Parent was in the stored trace but not in this response.
				rootPath = root.spanID
			} else {
				rootPath = root.parentID + "/" + root.spanID
			}
		} else {
			rootPath = root.spanID
		}
		h.dfs(root, rootPath, 1, maxDepth, childrenOf, returnedIDs, fullChildrenOf, &result)
	}
	return result
}

// dfs appends the current span to result and then recurses into its children.
// When maxDepth > 0 and the current span is at the depth limit, its children
// are counted but not visited, and TruncatedChildren is set on the emitted span.
// TruncatedChildren also counts direct children that exist in the stored trace
// but were omitted by the per-request span-count limit.
func (h *getTraceTopologyHandler) dfs(
	span *rawSpan,
	path string,
	depth int,
	maxDepth int,
	childrenOf map[string][]*rawSpan,
	returnedIDs map[string]struct{},
	fullChildrenOf map[string][]*rawSpan,
	result *[]types.TopologySpan,
) {
	if maxDepth > 0 && depth > maxDepth {
		return
	}

	truncated := 0
	if maxDepth > 0 && depth >= maxDepth {
		truncated = len(childrenOf[span.spanID])
	}
	for _, child := range fullChildrenOf[span.spanID] {
		if _, ok := returnedIDs[child.spanID]; !ok {
			truncated++
		}
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

	// Recurse only for children still in this response and above the depth limit.
	if maxDepth == 0 || depth < maxDepth {
		for _, child := range childrenOf[span.spanID] {
			h.dfs(child, path+"/"+child.spanID, depth+1, maxDepth, childrenOf, returnedIDs, fullChildrenOf, result)
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
