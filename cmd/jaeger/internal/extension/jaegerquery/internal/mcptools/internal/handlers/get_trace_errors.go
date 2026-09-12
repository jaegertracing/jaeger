// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// getTraceErrorsHandler implements the get_trace_errors MCP tool.
// This tool retrieves all spans with error status from a specific trace, returning full
// OTLP span details including attributes, events, and links for error analysis.
type getTraceErrorsHandler struct {
	queryService             queryServiceGetTracesInterface
	maxSpanDetailsPerRequest int
}

// NewGetTraceErrorsHandler creates a new get_trace_errors handler and returns the handler function.
func NewGetTraceErrorsHandler(
	queryService *querysvc.QueryService,
	maxSpanDetailsPerRequest int,
) mcp.ToolHandlerFor[types.GetTraceErrorsInput, types.GetTraceErrorsOutput] {
	h := &getTraceErrorsHandler{
		queryService:             queryService,
		maxSpanDetailsPerRequest: maxSpanDetailsPerRequest,
	}
	return h.handle
}

// errorCandidate holds raw data for a single error span before prioritization.
type errorCandidate struct {
	pos    jptrace.SpanIterPos
	span   ptrace.Span
	spanID string
	depth  int
}

// handle processes the get_trace_errors tool request.
//
// The response returns at most maxSpanDetailsPerRequest error spans, prioritized by tree
// depth (deepest first) so that the originating root-cause span is included even when the
// trace has more errors than the configured limit. Tie-breaking is by span ID (ascending)
// to make the selected subset deterministic regardless of storage or aggregation order.
func (h *getTraceErrorsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetTraceErrorsInput,
) (*mcp.CallToolResult, types.GetTraceErrorsOutput, error) {
	// Build query parameters (includes validation)
	params, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetTraceErrorsOutput{}, err
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// AggregateTraces reassembles the full trace so TotalErrorCount reflects every error span.
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Pass 1: collect all error candidates and a full parent map for depth computation.
	var candidates []errorCandidate
	// parentOf maps spanID → parentSpanID for every span in the trace (not just errors),
	// so that the depth of a deep error span is computed correctly even when its ancestors
	// are not themselves in error.
	parentOf := make(map[string]string)
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceErrorsOutput{}, err
		}

		traceFound = true

		for pos, span := range jptrace.SpanIter(trace) {
			sid := span.SpanID().String()
			pid := ""
			if !span.ParentSpanID().IsEmpty() {
				pid = span.ParentSpanID().String()
			}
			parentOf[sid] = pid

			if span.Status().Code() == ptrace.StatusCodeError {
				candidates = append(candidates, errorCandidate{
					pos:    pos,
					span:   span,
					spanID: sid,
				})
			}
		}
	}

	if !traceFound {
		return nil, types.GetTraceErrorsOutput{}, errors.New("trace not found")
	}

	// TotalErrorCount is the full count before any limit is applied.
	totalErrors := len(candidates)

	// Pass 2: compute depth for each error candidate using the full span tree.
	depthCache := make(map[string]int, len(parentOf))
	for i := range candidates {
		candidates[i].depth = spanDepth(candidates[i].spanID, parentOf, depthCache, 0)
	}

	// Sort deepest first; break ties by span ID (ascending) for determinism.
	slices.SortStableFunc(candidates, func(a, b errorCandidate) int {
		if a.depth != b.depth {
			return cmp.Compare(b.depth, a.depth) // descending depth
		}
		return cmp.Compare(a.spanID, b.spanID) // ascending spanID
	})

	// Apply the detail limit after sorting so the most actionable spans are retained.
	if h.maxSpanDetailsPerRequest > 0 && len(candidates) > h.maxSpanDetailsPerRequest {
		candidates = candidates[:h.maxSpanDetailsPerRequest]
	}

	errorSpans := make([]types.SpanDetail, 0, len(candidates))
	for _, c := range candidates {
		errorSpans = append(errorSpans, buildSpanDetail(c.pos, c.span))
	}

	output := types.GetTraceErrorsOutput{
		TraceID:         input.TraceID,
		TotalErrorCount: totalErrors,
		Spans:           errorSpans,
	}

	return nil, output, nil
}

// spanDepth returns the distance from the trace root to the span identified by spanID.
// Root spans (empty or absent parent) have depth 0. A visited counter guards against
// cycles in malformed traces; once the recursion depth exceeds the number of known spans,
// the span is treated as a root.
func spanDepth(spanID string, parentOf map[string]string, cache map[string]int, visited int) int {
	if d, ok := cache[spanID]; ok {
		return d
	}
	// Guard against cycles in malformed traces.
	if visited > len(parentOf) {
		cache[spanID] = 0
		return 0
	}
	pid, ok := parentOf[spanID]
	if !ok || pid == "" {
		cache[spanID] = 0
		return 0
	}
	d := 1 + spanDepth(pid, parentOf, cache, visited+1)
	cache[spanID] = d
	return d
}

// buildQuery converts GetTraceErrorsInput to querysvc.GetTraceParams.
func (*getTraceErrorsHandler) buildQuery(input types.GetTraceErrorsInput) (querysvc.GetTraceParams, error) {
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
