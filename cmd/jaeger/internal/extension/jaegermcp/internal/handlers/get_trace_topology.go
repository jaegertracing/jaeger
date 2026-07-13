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

	// AggregateTraces reassembles the full trace so TotalSpanCount reflects every span
	// and so we can build a correct DFS topology regardless of OTLP container ordering.
	// SpanIter walks resource/scope/span order, so a root span that appears after its
	// children would be dropped if we capped collection here, producing an incorrect
	// tree (children promoted to roots, paths broken). Instead, we collect every span,
	// build the full DFS-ordered topology, then truncate the resulting list — roots
	// are emitted first by DFS, so capping the tail removes leaves rather than roots.
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
		if maxSpans > 0 && len(result) >= maxSpans {
			break
		}
		// For orphans (has a parentID but parent not in trace), prepend the missing
		// parent ID to the path so the caller can identify the attachment point.
		var rootPath string
		if root.parentID != "" {
			rootPath = root.parentID + "/" + root.spanID
		} else {
			rootPath = root.spanID
		}
		h.dfs(root, rootPath, 1, maxDepth, maxSpans, childrenOf, &result)
	}
	return result
}

// dfs appends the current span to result and then recurses into its children.
// When maxDepth > 0 and the current span is at the depth limit, its children
// are counted but not visited, and TruncatedChildren is set on the emitted span.
// When maxSpans > 0, the walk stops once result holds that many spans; since the
// walk is roots-first, this keeps the roots and drops the tail (leaf) spans.
func (h *getTraceTopologyHandler) dfs(
	span *rawSpan,
	path string,
	depth int,
	maxDepth int,
	maxSpans int,
	childrenOf map[string][]*rawSpan,
	result *[]types.TopologySpan,
) {
	if maxDepth > 0 && depth > maxDepth {
		return
	}
	if maxSpans > 0 && len(*result) >= maxSpans {
		return
	}

	// Count and remove children beyond maxDepth
	truncated := 0
	if maxDepth > 0 && depth >= maxDepth {
		truncated = len(childrenOf[span.spanID])
	}

	*result = append(*result, toTopologySpan(span, path, truncated))

	// Recursively process children if above the depth limit
	if truncated == 0 {
		for _, child := range childrenOf[span.spanID] {
			h.dfs(child, path+"/"+child.spanID, depth+1, maxDepth, maxSpans, childrenOf, result)
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
