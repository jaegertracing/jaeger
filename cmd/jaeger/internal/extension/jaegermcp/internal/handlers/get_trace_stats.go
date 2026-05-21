// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// getTraceStatsHandler implements the get_trace_stats MCP tool.
// It accepts the same filters as search_traces but instead of returning individual
// trace summaries it computes aggregate statistics (count, error rate, latency
// percentiles, span counts, top services) across all matching traces.
//
// This is particularly useful for LLM-based agents that need a high-level
// understanding of a service's health before deciding which individual traces
// to inspect in detail.
type getTraceStatsHandler struct {
	queryService queryServiceInterface
	maxResults   int
}

// NewGetTraceStatsHandler creates a new get_trace_stats handler and returns the handler function.
func NewGetTraceStatsHandler(
	queryService *querysvc.QueryService,
	maxResults int,
) mcp.ToolHandlerFor[types.GetTraceStatsInput, types.GetTraceStatsOutput] {
	h := &getTraceStatsHandler{
		queryService: queryService,
		maxResults:   maxResults,
	}
	return h.handle
}

// handle processes the get_trace_stats tool request.
func (h *getTraceStatsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetTraceStatsInput,
) (*mcp.CallToolResult, types.GetTraceStatsOutput, error) {
	query, err := buildStatsQuery(input, h.maxResults)
	if err != nil {
		return nil, types.GetTraceStatsOutput{}, err
	}

	// Use the effective search depth computed by buildStatsQuery (already clamped
	// to maxResults and defaulted) as the loop limit for deterministic behaviour.
	effectiveLimit := query.TraceQueryParams.SearchDepth

	tracesIter := h.queryService.FindTraces(ctx, query)
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Pre-allocate with capacity to avoid repeated reallocations during iteration.
	var durations []int64
	var spanCounts []int
	if effectiveLimit > 0 {
		durations = make([]int64, 0, effectiveLimit)
		spanCounts = make([]int, 0, effectiveLimit)
	}
	serviceCounts := make(map[string]int)
	errorCount := 0

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceStatsOutput{}, fmt.Errorf("error reading traces: %w", err)
		}

		summary := buildTraceSummary(trace)

		durations = append(durations, summary.DurationUs)
		spanCounts = append(spanCounts, summary.SpanCount)

		if summary.HasErrors {
			errorCount++
		}

		for _, svc := range summary.Services {
			serviceCounts[svc]++
		}

		if effectiveLimit > 0 && len(durations) >= effectiveLimit {
			break
		}
	}

	traceCount := len(durations)
	if traceCount == 0 {
		// Return a zero-value output rather than an error — no traces is a valid result.
		return nil, types.GetTraceStatsOutput{}, nil
	}

	output := types.GetTraceStatsOutput{
		TraceCount:    traceCount,
		ErrorCount:    errorCount,
		ErrorRate:     float64(errorCount) / float64(traceCount),
		DurationStats: computeDurationStats(durations),
		SpanStats:     computeSpanStats(spanCounts),
		TopServices:   buildTopServices(serviceCounts),
	}

	return nil, output, nil
}

// buildStatsQuery converts GetTraceStatsInput to querysvc.TraceQueryParams.
// It reuses buildTraceQueryParams, the same standalone helper used by search_traces,
// ensuring consistent filter parsing across both tools.
func buildStatsQuery(input types.GetTraceStatsInput, maxResults int) (querysvc.TraceQueryParams, error) {
	return buildTraceQueryParams(types.SearchTracesInput{
		StartTimeMin: input.StartTimeMin,
		StartTimeMax: input.StartTimeMax,
		ServiceName:  input.ServiceName,
		SpanName:     input.SpanName,
		Attributes:   input.Attributes,
		WithErrors:   input.WithErrors,
		DurationMin:  input.DurationMin,
		DurationMax:  input.DurationMax,
		SearchDepth:  input.SearchDepth,
	}, maxResults)
}

// computeDurationStats calculates min, max, mean, p50, p95, and p99 over a
// slice of durations (in microseconds). The input slice is sorted in-place.
func computeDurationStats(durations []int64) types.DurationStats {
	if len(durations) == 0 {
		return types.DurationStats{}
	}

	slices.Sort(durations)

	n := len(durations)
	var sum int64
	for _, d := range durations {
		sum += d
	}

	return types.DurationStats{
		MinUs:  durations[0],
		MaxUs:  durations[n-1],
		MeanUs: sum / int64(n),
		P50Us:  percentile(durations, 50),
		P95Us:  percentile(durations, 95),
		P99Us:  percentile(durations, 99),
	}
}

// percentile returns the p-th percentile value from a pre-sorted slice using
// the nearest-rank method (ceiling index).
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// computeSpanStats calculates min, max, and mean span counts across traces.
func computeSpanStats(counts []int) types.SpanStats {
	if len(counts) == 0 {
		return types.SpanStats{}
	}

	minSpans := counts[0]
	maxSpans := counts[0]
	total := 0

	for _, c := range counts {
		if c < minSpans {
			minSpans = c
		}
		if c > maxSpans {
			maxSpans = c
		}
		total += c
	}

	return types.SpanStats{
		MinSpans:  minSpans,
		MaxSpans:  maxSpans,
		MeanSpans: float64(total) / float64(len(counts)),
	}
}

// buildTopServices converts the service->traceCount map into a slice sorted by
// descending trace count, then alphabetically by service name for ties.
// The result is capped at 10 entries to keep the LLM context compact.
func buildTopServices(serviceCounts map[string]int) []types.ServiceCount {
	const maxTopServices = 10

	result := make([]types.ServiceCount, 0, len(serviceCounts))
	for svc, count := range serviceCounts {
		result = append(result, types.ServiceCount{
			Service:    svc,
			TraceCount: count,
		})
	}

	slices.SortStableFunc(result, func(a, b types.ServiceCount) int {
		if a.TraceCount > b.TraceCount {
			return -1
		}
		if a.TraceCount < b.TraceCount {
			return 1
		}
		if a.Service < b.Service {
			return -1
		}
		if a.Service > b.Service {
			return 1
		}
		return 0
	})

	if len(result) > maxTopServices {
		result = result[:maxTopServices]
	}
	return result
}
