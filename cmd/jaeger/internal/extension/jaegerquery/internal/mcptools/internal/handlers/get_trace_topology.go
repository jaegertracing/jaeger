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
	spanID    string
	parentID  string // empty string if this is a root span
	startNano int64  // used for sorting children by start time
	// pos and span reference the source span so the detail fields (service,
	// name, timing, status) can be materialized lazily in toTopologySpan, only
	// for the spans that survive the per-request cap rather than for every span.
	pos  jptrace.SpanIterPos
	span ptrace.Span
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

	// AggregateTraces reassembles the full trace so TotalSpanCount counts every span and
	// the tree is built from all of them. Capping here, in container order, would drop a
	// root stored after its children and promote them to roots. The per-request cap is
	// applied later in the DFS walk, which keeps a DFS prefix: later roots and later
	// sibling subtrees are omitted, and TruncatedChildren only reports the depth cut.
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

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

	totalSpans := len(spans)
	topology := h.buildFlatTopology(spans, input.Depth, h.maxSpanDetailsPerRequest)

	output := types.GetTraceTopologyOutput{
		TraceID:        input.TraceID,
		TotalSpanCount: totalSpans,
		Spans:          topology,
	}

	return nil, output, nil
}

// buildQuery converts GetTraceTopologyInput to querysvc.GetTraceParams.
func (*getTraceTopologyHandler) buildQuery(input types.GetTraceTopologyInput) (querysvc.GetTraceParams, error) {
	// Validate input
	if input.TraceID == "" {
		return querysvc.GetTraceParams{}, errors.New("trace_id is required")
	}

	traceID, err := jptrace.TraceIDFromString(input.TraceID)
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

// extractRawSpan captures the structural fields needed to build the topology
// tree and keeps a reference to the source span so its detail fields can be
// materialized later, only for the spans that are actually emitted.
func extractRawSpan(pos jptrace.SpanIterPos, span ptrace.Span) rawSpan {
	parentSpanID := ""
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().String()
	}

	return rawSpan{
		spanID:    span.SpanID().String(),
		parentID:  parentSpanID,
		startNano: span.StartTimestamp().AsTime().UnixNano(),
		pos:       pos,
		span:      span,
	}
}

// toTopologySpan materializes the detail fields for a span being emitted into
// the topology output. This is deliberately deferred from extractRawSpan so the
// work is only done for spans that survive the per-request cap.
func toTopologySpan(rs *rawSpan, path string, truncatedChildren int) types.TopologySpan {
	serviceName := ""
	if svc, ok := rs.pos.Resource.Resource().Attributes().Get("service.name"); ok {
		serviceName = svc.Str()
	}
	duration := rs.span.EndTimestamp().AsTime().Sub(rs.span.StartTimestamp().AsTime())
	return types.TopologySpan{
		Path:              path,
		Service:           serviceName,
		SpanName:          rs.span.Name(),
		StartTime:         rs.span.StartTimestamp().AsTime().Format(time.RFC3339Nano),
		DurationUs:        duration.Microseconds(),
		Status:            rs.span.Status().Code().String(),
		TruncatedChildren: truncatedChildren,
	}
}

// buildFlatTopology converts a flat slice of rawSpans into a depth-first ordered
// slice of TopologySpan where each span's Path encodes its ancestry as a
// slash-delimited sequence of span IDs from the root down to that span.
// Orphan spans (whose parent is absent from the trace) have their missing parent ID
// prepended to the path so the caller can identify the attachment point.
// When maxDepth > 0, spans beyond that depth are omitted and the last included
// ancestor records the count of excluded direct children in TruncatedChildren.
func (h *getTraceTopologyHandler) buildFlatTopology(spans []rawSpan, maxDepth, maxSpans int) []types.TopologySpan {
	n := len(spans)
	childrenOf := make([][]int, n)
	var roots []int
	for i := range spans {
		parentIdx := parentIndex(spans, i)
		if parentIdx >= 0 {
			childrenOf[parentIdx] = append(childrenOf[parentIdx], i)
		} else {
			roots = append(roots, i)
		}
	}

	sortIndicesByStartNano(spans, roots)
	for i := range childrenOf {
		sortIndicesByStartNano(spans, childrenOf[i])
	}

	capacity := n
	if maxSpans > 0 && maxSpans < capacity {
		capacity = maxSpans
	}
	result := make([]types.TopologySpan, 0, capacity)
	visited := make([]bool, n)
	reachable := reachableFromRoots(n, roots, childrenOf)

	for _, rootIdx := range roots {
		if maxSpans > 0 && len(result) >= maxSpans {
			break
		}
		result = h.emitFromRoot(spans, rootIdx, maxDepth, maxSpans, childrenOf, visited, result, false)
	}

	var unreachable []int
	for i := range spans {
		if !reachable[i] {
			unreachable = append(unreachable, i)
		}
	}
	sortIndicesByStartNano(spans, unreachable)
	for _, rootIdx := range unreachable {
		if maxSpans > 0 && len(result) >= maxSpans {
			break
		}
		result = h.emitFromRoot(spans, rootIdx, maxDepth, maxSpans, childrenOf, visited, result, true)
	}
	return result
}

func reachableFromRoots(n int, roots []int, childrenOf [][]int) []bool {
	reachable := make([]bool, n)
	var walk func(idx int)
	walk = func(idx int) {
		if reachable[idx] {
			return
		}
		reachable[idx] = true
		for _, childIdx := range childrenOf[idx] {
			walk(childIdx)
		}
	}
	for _, rootIdx := range roots {
		walk(rootIdx)
	}
	return reachable
}

// parentIndex returns the slice index of the parent span, or -1 when the span
// has no parent or the parent ID does not appear in the trace. When several
// spans share the same ID, the earliest in slice order is treated as the parent
// (matching OTLP parent references that only carry a span ID).
func parentIndex(spans []rawSpan, childIdx int) int {
	parentID := spans[childIdx].parentID
	if parentID == "" {
		return -1
	}
	for i := range spans {
		if spans[i].spanID == parentID {
			return i
		}
	}
	return -1
}

func (h *getTraceTopologyHandler) emitFromRoot(
	spans []rawSpan,
	rootIdx int,
	maxDepth, maxSpans int,
	childrenOf [][]int,
	visited []bool,
	result []types.TopologySpan,
	unreachableEntry bool,
) []types.TopologySpan {
	rootPath := topologyRootPath(spans, rootIdx, unreachableEntry)
	h.dfs(spans, rootIdx, rootPath, 1, maxDepth, maxSpans, childrenOf, visited, &result)
	return result
}

func topologyRootPath(spans []rawSpan, idx int, unreachableEntry bool) string {
	s := &spans[idx]
	if s.parentID == "" {
		return s.spanID
	}
	if unreachableEntry || parentIndex(spans, idx) < 0 {
		return s.parentID + "/" + s.spanID
	}
	return s.spanID
}

// dfs appends the current span to result and then recurses into its children.
// When maxDepth > 0 and the current span is at the depth limit, its children
// are counted but not visited, and TruncatedChildren is set on the emitted span.
// When maxSpans > 0, callers stop the walk once result holds that many spans, so the
// output is a DFS prefix: later roots and later sibling subtrees are omitted, while
// TruncatedChildren stays the depth signal.
func (h *getTraceTopologyHandler) dfs(
	spans []rawSpan,
	spanIdx int,
	path string,
	depth int,
	maxDepth int,
	maxSpans int,
	childrenOf [][]int,
	visited []bool,
	result *[]types.TopologySpan,
) {
	if visited[spanIdx] {
		return
	}
	if maxDepth > 0 && depth > maxDepth {
		return
	}

	span := &spans[spanIdx]
	truncated := 0
	if maxDepth > 0 && depth >= maxDepth {
		truncated = len(childrenOf[spanIdx])
	}

	visited[spanIdx] = true
	*result = append(*result, toTopologySpan(span, path, truncated))

	if truncated == 0 {
		for _, childIdx := range childrenOf[spanIdx] {
			if maxSpans > 0 && len(*result) >= maxSpans {
				break
			}
			child := &spans[childIdx]
			h.dfs(spans, childIdx, path+"/"+child.spanID, depth+1, maxDepth, maxSpans, childrenOf, visited, result)
		}
	}
}

func sortIndicesByStartNano(spans []rawSpan, indices []int) {
	slices.SortStableFunc(indices, func(a, b int) int {
		sa, sb := spans[a], spans[b]
		if sa.startNano != sb.startNano {
			return cmp.Compare(sa.startNano, sb.startNano)
		}
		return cmp.Compare(sa.spanID, sb.spanID)
	})
}
