// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

func TestGetTraceStatsHandler_Handle_BasicStats(t *testing.T) {
	ids := uniqueTraceIDs(2)

	traceOK := createTestTrace(ids[0], "frontend", "/api/ok", false)
	traceErr := createTestTrace(ids[1], "frontend", "/api/err", true)

	mock := newMockFindTraces(traceOK, traceErr)
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 100}

	input := types.GetTraceStatsInput{ServiceName: "frontend"}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, 2, output.TraceCount)
	assert.Equal(t, 1, output.ErrorCount)
	assert.InDelta(t, 0.5, output.ErrorRate, 0.001)
	assert.Equal(t, 1, output.SpanStats.MinSpans)
	assert.Equal(t, 1, output.SpanStats.MaxSpans)
	assert.InDelta(t, 1.0, output.SpanStats.MeanSpans, 0.001)
	require.NotEmpty(t, output.TopServices)
	assert.Equal(t, "frontend", output.TopServices[0].Service)
	assert.Equal(t, 2, output.TopServices[0].TraceCount)
}

func TestGetTraceStatsHandler_Handle_NoTraces(t *testing.T) {
	mock := newMockFindTraces() // yields nothing
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 100}

	input := types.GetTraceStatsInput{ServiceName: "nonexistent"}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, 0, output.TraceCount)
	assert.Equal(t, 0, output.ErrorCount)
	assert.InDelta(t, 0.0, output.ErrorRate, 0.001)
}

func TestGetTraceStatsHandler_Handle_AllErrors(t *testing.T) {
	ids := uniqueTraceIDs(3)

	trace1 := createTestTrace(ids[0], "svc", "/op1", true)
	trace2 := createTestTrace(ids[1], "svc", "/op2", true)
	trace3 := createTestTrace(ids[2], "svc", "/op3", true)

	mock := newMockFindTraces(trace1, trace2, trace3)
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 100}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{ServiceName: "svc"})

	require.NoError(t, err)
	assert.Equal(t, 3, output.TraceCount)
	assert.Equal(t, 3, output.ErrorCount)
	assert.InDelta(t, 1.0, output.ErrorRate, 0.001)
}

func TestGetTraceStatsHandler_Handle_DurationStats(t *testing.T) {
	ids := uniqueTraceIDs(2)

	trace1 := createTestTrace(ids[0], "svc", "/op1", false)
	trace2 := createTestTrace(ids[1], "svc", "/op2", false)

	mock := newMockFindTraces(trace1, trace2)
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 100}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{ServiceName: "svc"})

	require.NoError(t, err)
	assert.Equal(t, 2, output.TraceCount)
	assert.GreaterOrEqual(t, output.DurationStats.MinUs, int64(0))
	assert.GreaterOrEqual(t, output.DurationStats.MaxUs, output.DurationStats.MinUs)
	assert.GreaterOrEqual(t, output.DurationStats.MeanUs, output.DurationStats.MinUs)
	assert.GreaterOrEqual(t, output.DurationStats.P50Us, output.DurationStats.MinUs)
	assert.GreaterOrEqual(t, output.DurationStats.P95Us, output.DurationStats.P50Us)
	assert.GreaterOrEqual(t, output.DurationStats.P99Us, output.DurationStats.P95Us)
}

func TestGetTraceStatsHandler_Handle_TopServicesOrdering(t *testing.T) {
	ids := uniqueTraceIDs(4)
	idx := 0

	// Build traces with explicit service names.
	makeSvcTrace := func(svc string) ptrace.Traces {
		tr := createTestTrace(ids[idx], svc, "/op", false)
		idx++
		return tr
	}

	// frontend: 3 traces, backend: 1 trace
	mock := newMockFindTraces(
		makeSvcTrace("frontend"),
		makeSvcTrace("frontend"),
		makeSvcTrace("frontend"),
		makeSvcTrace("backend"),
	)
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 100}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{ServiceName: "frontend"})

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(output.TopServices), 1)
	assert.Equal(t, "frontend", output.TopServices[0].Service)
	assert.Equal(t, 3, output.TopServices[0].TraceCount)
}

func TestGetTraceStatsHandler_Handle_LimitEnforced(t *testing.T) {
	ids := uniqueTraceIDs(5)

	mock := newMockFindTraces(
		createTestTrace(ids[0], "svc", "/op1", false),
		createTestTrace(ids[1], "svc", "/op2", false),
		createTestTrace(ids[2], "svc", "/op3", false),
		createTestTrace(ids[3], "svc", "/op4", false),
		createTestTrace(ids[4], "svc", "/op5", false),
	)
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 3}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{ServiceName: "svc"})

	require.NoError(t, err)
	assert.Equal(t, 3, output.TraceCount)
}

func TestGetTraceStatsHandler_Handle_QueryError(t *testing.T) {
	mock := newMockFindTracesError(errors.New("storage unavailable"))
	handler := &getTraceStatsHandler{queryService: mock, maxResults: 100}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{ServiceName: "svc"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage unavailable")
}

func TestGetTraceStatsHandler_Handle_MissingServiceName(t *testing.T) {
	handler := NewGetTraceStatsHandler(nil, 100)

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name is required")
}

func TestGetTraceStatsHandler_Handle_InvalidTimeFormat(t *testing.T) {
	handler := NewGetTraceStatsHandler(nil, 100)

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, types.GetTraceStatsInput{
		ServiceName:  "svc",
		StartTimeMin: "not-a-time",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_time_min")
}

// --- unit tests for helper functions ---

func TestComputeDurationStats_Empty(t *testing.T) {
	stats := computeDurationStats(nil)
	assert.Equal(t, types.DurationStats{}, stats)
}

func TestComputeDurationStats_Single(t *testing.T) {
	stats := computeDurationStats([]int64{1000})
	assert.Equal(t, int64(1000), stats.MinUs)
	assert.Equal(t, int64(1000), stats.MaxUs)
	assert.Equal(t, int64(1000), stats.MeanUs)
	assert.Equal(t, int64(1000), stats.P50Us)
	assert.Equal(t, int64(1000), stats.P95Us)
	assert.Equal(t, int64(1000), stats.P99Us)
}

func TestComputeDurationStats_Multiple(t *testing.T) {
	// 10 values: 100, 200, ..., 1000 (shuffled)
	durations := []int64{500, 100, 900, 300, 700, 200, 800, 400, 600, 1000}
	stats := computeDurationStats(durations)

	assert.Equal(t, int64(100), stats.MinUs)
	assert.Equal(t, int64(1000), stats.MaxUs)
	assert.Equal(t, int64(550), stats.MeanUs)
	// nearest-rank p50 of [100,200,300,400,500,600,700,800,900,1000]:
	// ceil(50/100*10)-1 = ceil(5)-1 = 4 → sorted[4] = 500
	assert.Equal(t, int64(500), stats.P50Us)
}

func TestComputeSpanStats_Empty(t *testing.T) {
	stats := computeSpanStats(nil)
	assert.Equal(t, types.SpanStats{}, stats)
}

func TestComputeSpanStats_Values(t *testing.T) {
	stats := computeSpanStats([]int{1, 5, 3, 2, 4})
	assert.Equal(t, 1, stats.MinSpans)
	assert.Equal(t, 5, stats.MaxSpans)
	assert.InDelta(t, 3.0, stats.MeanSpans, 0.001)
}

func TestBuildTopServices_Empty(t *testing.T) {
	result := buildTopServices(map[string]int{})
	assert.Empty(t, result)
}

func TestBuildTopServices_Ordering(t *testing.T) {
	counts := map[string]int{
		"alpha": 5,
		"beta":  10,
		"gamma": 5,
	}
	result := buildTopServices(counts)

	require.Len(t, result, 3)
	assert.Equal(t, "beta", result[0].Service)
	assert.Equal(t, 10, result[0].TraceCount)
	// alpha before gamma (same count, alphabetical)
	assert.Equal(t, "alpha", result[1].Service)
	assert.Equal(t, "gamma", result[2].Service)
}

func TestBuildTopServices_CappedAt10(t *testing.T) {
	counts := make(map[string]int)
	for i := 0; i < 15; i++ {
		counts[string(rune('a'+i))] = i + 1
	}
	result := buildTopServices(counts)
	assert.Len(t, result, 10)
}

func TestPercentile_Empty(t *testing.T) {
	assert.Equal(t, int64(0), percentile(nil, 50))
}

func TestPercentile_Boundary(t *testing.T) {
	sorted := []int64{10, 20, 30, 40, 50}
	assert.Equal(t, int64(10), percentile(sorted, 0))
	assert.Equal(t, int64(50), percentile(sorted, 100))
}
