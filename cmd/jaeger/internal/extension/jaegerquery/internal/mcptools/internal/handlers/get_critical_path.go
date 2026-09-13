// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/criticalpath"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// queryServiceGetCriticalPathInterface defines the interface we need from QueryService for get_critical_path
type queryServiceGetCriticalPathInterface interface {
	GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
}

// getCriticalPathHandler implements the get_critical_path MCP tool.
// This tool identifies the sequence of spans forming the critical latency path
// (the blocking execution path) in a distributed trace.
type getCriticalPathHandler struct {
	queryService             queryServiceGetCriticalPathInterface
	maxSpanDetailsPerRequest int
}

// NewGetCriticalPathHandler creates a new get_critical_path handler and returns the handler function.
func NewGetCriticalPathHandler(
	queryService *querysvc.QueryService,
	maxSpanDetailsPerRequest int,
) mcp.ToolHandlerFor[types.GetCriticalPathInput, types.GetCriticalPathOutput] {
	h := &getCriticalPathHandler{
		queryService:             queryService,
		maxSpanDetailsPerRequest: maxSpanDetailsPerRequest,
	}
	return h.handle
}

// handle processes the get_critical_path tool request.
func (h *getCriticalPathHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetCriticalPathInput,
) (*mcp.CallToolResult, types.GetCriticalPathOutput, error) {
	// Build query parameters (includes validation)
	params, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetCriticalPathOutput{}, err
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// Wrap with AggregateTraces to ensure each ptrace.Traces contains a complete trace
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	var trace ptrace.Traces
	traceFound := false

	for t, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetCriticalPathOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		traceFound = true
		trace = t
		break // We expect only one trace since we're querying by a single trace ID
	}

	if !traceFound {
		return nil, types.GetCriticalPathOutput{}, errors.New("trace not found")
	}

	// Compute critical path
	criticalPathSections, err := criticalpath.ComputeCriticalPathFromTraces(trace)
	if err != nil {
		return nil, types.GetCriticalPathOutput{}, fmt.Errorf("failed to compute critical path: %w", err)
	}

	// Build output
	output := h.buildOutput(input.TraceID, trace, criticalPathSections)

	return nil, output, nil
}

// buildQuery converts GetCriticalPathInput to querysvc.GetTraceParams.
func (*getCriticalPathHandler) buildQuery(input types.GetCriticalPathInput) (querysvc.GetTraceParams, error) {
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

// buildOutput constructs the GetCriticalPathOutput from the trace and critical path sections.
func (h *getCriticalPathHandler) buildOutput(
	traceIDStr string,
	trace ptrace.Traces,
	criticalPathSections []criticalpath.Section,
) types.GetCriticalPathOutput {
	// Build a map of spans for quick lookup
	spanMap := jptrace.SpanMap(trace, func(span ptrace.Span) string {
		return span.SpanID().String()
	})

	// Build a map of span ID to service name
	serviceMap := make(map[string]string)
	var traceStartTime uint64
	var traceEndTime uint64

	for i := 0; i < trace.ResourceSpans().Len(); i++ {
		rs := trace.ResourceSpans().At(i)
		serviceName := "unknown"
		if serviceNameAttr, ok := rs.Resource().Attributes().Get("service.name"); ok {
			serviceName = serviceNameAttr.Str()
		}

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				serviceMap[span.SpanID().String()] = serviceName

				// Track trace start and end times
				startTime := uint64(span.StartTimestamp()) / 1000 // Convert to microseconds
				endTime := uint64(span.EndTimestamp()) / 1000

				if traceStartTime == 0 || startTime < traceStartTime {
					traceStartTime = startTime
				}
				if endTime > traceEndTime {
					traceEndTime = endTime
				}
			}
		}
	}

	// Convert critical path sections to output format
	segments := make([]types.CriticalPathSegment, 0, len(criticalPathSections))
	var criticalPathDuration uint64

	for _, section := range criticalPathSections {
		span, ok := spanMap[section.SpanID]
		if !ok {
			continue // Skip if span not found
		}

		// Get service name from service map
		serviceName := serviceMap[section.SpanID]

		selfTime := section.SectionEnd - section.SectionStart
		criticalPathDuration += selfTime

		segments = append(segments, types.CriticalPathSegment{
			SpanID:        section.SpanID,
			Service:       serviceName,
			SpanName:      span.Name(),
			SelfTimeUs:    selfTime,
			StartOffsetUs: section.SectionStart - traceStartTime,
			EndOffsetUs:   section.SectionEnd - traceStartTime,
		})
	}

	totalSegmentCount := len(segments)
	segments = h.capSegments(segments)

	return types.GetCriticalPathOutput{
		TraceID:                traceIDStr,
		TotalDurationUs:        traceEndTime - traceStartTime,
		CriticalPathDurationUs: criticalPathDuration,
		TotalSegmentCount:      totalSegmentCount,
		Segments:               segments,
	}
}

// capSegments bounds the number of segments returned to the caller. The critical
// path and its total duration are always computed over the full trace (see
// buildOutput); only the segments slice itself is capped here, so truncating a
// deep trace can never change which spans are considered part of the critical
// path. When capping is needed, the segments with the highest self_time_us are
// kept, since those are the ones the tool description tells callers to look at
// first, and a stable sort preserves their original chronological order among
// ties. segments is freshly built by the caller for this response and held
// nowhere else, so it is safe to sort in place instead of cloning.
func (h *getCriticalPathHandler) capSegments(segments []types.CriticalPathSegment) []types.CriticalPathSegment {
	if h.maxSpanDetailsPerRequest <= 0 || len(segments) <= h.maxSpanDetailsPerRequest {
		return segments
	}

	slices.SortStableFunc(segments, func(a, b types.CriticalPathSegment) int {
		return cmp.Compare(b.SelfTimeUs, a.SelfTimeUs) // descending
	})
	return segments[:h.maxSpanDetailsPerRequest]
}
