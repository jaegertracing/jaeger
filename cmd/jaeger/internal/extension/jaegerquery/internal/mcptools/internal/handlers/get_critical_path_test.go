// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/criticalpath"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// mockGetCriticalPathQueryService is a mock implementation for testing get_critical_path
type mockGetCriticalPathQueryService struct {
	traces []ptrace.Traces
	err    error
}

func (m *mockGetCriticalPathQueryService) GetTraces(
	_ context.Context,
	_ querysvc.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		if m.err != nil {
			yield(nil, m.err)
			return
		}
		yield(m.traces, nil)
	}
}

// createCriticalPathTestTrace creates a simple test trace with critical path
func createCriticalPathTestTrace() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()

	// Root span
	rootSpan := ss.Spans().AppendEmpty()
	rootSpan.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	rootSpan.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	rootSpan.SetStartTimestamp(pcommon.Timestamp(1000 * 1000)) // 1ms in nanoseconds
	rootSpan.SetEndTimestamp(pcommon.Timestamp(101000 * 1000)) // 101ms
	rootSpan.SetName("root-operation")

	// Child span
	childSpan := ss.Spans().AppendEmpty()
	childSpan.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	childSpan.SetParentSpanID(rootSpan.SpanID())
	childSpan.SetTraceID(rootSpan.TraceID())
	childSpan.SetStartTimestamp(pcommon.Timestamp(20000 * 1000)) // 20ms
	childSpan.SetEndTimestamp(pcommon.Timestamp(40000 * 1000))   // 40ms
	childSpan.SetName("child-operation")

	return traces
}

func TestNewGetCriticalPathHandler(t *testing.T) {
	// We can pass nil because we only check if it returns a handler function
	handler := NewGetCriticalPathHandler(nil, 20)
	assert.NotNil(t, handler)
}

// buildNestedChainTrace returns a strictly nested chain of n+1 spans: span i
// covers [i ms, (2n+2-i) ms] and is the parent of span i+1, so every child is
// fully inside its parent. The critical path walk then produces a section on
// both sides of each child plus the innermost span, more than 2n sections in
// total, without relying on the sibling-walk behavior of the algorithm.
func buildNestedChainTrace(n int) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	ss := rs.ScopeSpans().AppendEmpty()

	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{0xFF}))
	span.SetName("root")
	const base = 1_000_000 // 1ms; a zero start trips buildOutput's traceStartTime==0 guard
	span.SetStartTimestamp(pcommon.Timestamp(base))
	span.SetEndTimestamp(pcommon.Timestamp(base + uint64(2*n+2)*1_000_000))

	parentID := span.SpanID()
	for i := 1; i <= n; i++ {
		child := ss.Spans().AppendEmpty()
		child.SetTraceID(span.TraceID())
		child.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2}))
		child.SetParentSpanID(parentID)
		child.SetName("child")
		child.SetStartTimestamp(pcommon.Timestamp(base + uint64(i)*1_000_000))
		child.SetEndTimestamp(pcommon.Timestamp(base + uint64(2*n+2-i)*1_000_000))
		parentID = child.SpanID()
	}
	return traces
}

func TestGetCriticalPathHandler_SegmentCap_KeepsLargestSelfTime(t *testing.T) {
	const spans, limit = 30, 5
	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{buildNestedChainTrace(spans)},
	}
	handler := &getCriticalPathHandler{queryService: mockQS, maxSegments: limit}

	_, output, err := handler.handle(context.Background(), nil, types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	})
	require.NoError(t, err)

	require.Len(t, output.Segments, limit)
	assert.Greater(t, output.TotalSegmentCount, limit,
		"total must reflect the uncapped path so truncation is detectable")

	// Survivors are the largest-self-time segments: every returned segment must
	// have self time >= the largest segment that was dropped.
	uncapped := &getCriticalPathHandler{queryService: mockQS, maxSegments: 0}
	_, full, err := uncapped.handle(context.Background(), nil, types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	})
	require.NoError(t, err)
	require.Len(t, full.Segments, full.TotalSegmentCount)
	assert.Equal(t, full.TotalSegmentCount, output.TotalSegmentCount)
	assert.Equal(t, full.CriticalPathDurationUs, output.CriticalPathDurationUs,
		"critical path duration must be computed over the full path")

	kept := make(map[string]bool, limit)
	minKept := uint64(math.MaxUint64)
	for _, s := range output.Segments {
		kept[s.SpanID+"/"+strconv.FormatUint(s.StartOffsetUs, 10)] = true
		if s.SelfTimeUs < minKept {
			minKept = s.SelfTimeUs
		}
	}
	for _, s := range full.Segments {
		if !kept[s.SpanID+"/"+strconv.FormatUint(s.StartOffsetUs, 10)] {
			assert.LessOrEqual(t, s.SelfTimeUs, minKept,
				"a dropped segment must not out-rank a kept one")
		}
	}

	// Returned segments keep the path's native order: trace end first,
	// matching the uncapped response (see section order in criticalpath tests).
	for i := 1; i < len(output.Segments); i++ {
		assert.GreaterOrEqual(t, output.Segments[i-1].StartOffsetUs, output.Segments[i].StartOffsetUs)
	}
	for i := 1; i < len(full.Segments); i++ {
		assert.GreaterOrEqual(t, full.Segments[i-1].StartOffsetUs, full.Segments[i].StartOffsetUs,
			"uncapped output must have the same ordering the cap preserves")
	}
}

func TestCapSegments_DeterministicTiebreaks(t *testing.T) {
	// Equal self times force the start-offset tiebreak, and equal start offsets
	// force the span-ID tiebreak, in both sort passes.
	segs := []types.CriticalPathSegment{
		{SpanID: "dd", SelfTimeUs: 10, StartOffsetUs: 40},
		{SpanID: "bb", SelfTimeUs: 10, StartOffsetUs: 20},
		{SpanID: "aa", SelfTimeUs: 10, StartOffsetUs: 20},
		{SpanID: "cc", SelfTimeUs: 5, StartOffsetUs: 30},
	}
	h := &getCriticalPathHandler{maxSegments: 3}

	got := h.capSegments(slices.Clone(segs))

	// The three self-time-10 segments survive; cc (5us) is dropped. Survivors
	// keep the native trace-end-first order, with the aa/bb tie at offset 20
	// broken by span ID.
	want := []types.CriticalPathSegment{
		{SpanID: "dd", SelfTimeUs: 10, StartOffsetUs: 40},
		{SpanID: "aa", SelfTimeUs: 10, StartOffsetUs: 20},
		{SpanID: "bb", SelfTimeUs: 10, StartOffsetUs: 20},
	}
	assert.Equal(t, want, got)

	// Same input in a different order produces the identical result.
	shuffled := []types.CriticalPathSegment{segs[3], segs[0], segs[2], segs[1]}
	assert.Equal(t, want, h.capSegments(shuffled))
}

func TestGetCriticalPathHandler_SegmentCap_ZeroMeansUnlimited(t *testing.T) {
	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{buildNestedChainTrace(30)},
	}
	handler := &getCriticalPathHandler{queryService: mockQS, maxSegments: 0}

	_, output, err := handler.handle(context.Background(), nil, types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	})
	require.NoError(t, err)
	assert.Len(t, output.Segments, output.TotalSegmentCount)
	assert.Greater(t, output.TotalSegmentCount, 20)
}

func TestGetCriticalPathHandler_SegmentCap_UnderLimitUntouched(t *testing.T) {
	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{createCriticalPathTestTrace()},
	}
	handler := &getCriticalPathHandler{queryService: mockQS, maxSegments: 20}

	_, output, err := handler.handle(context.Background(), nil, types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	})
	require.NoError(t, err)
	assert.Len(t, output.Segments, output.TotalSegmentCount)
	assert.LessOrEqual(t, output.TotalSegmentCount, 20)
}

func TestGetCriticalPathHandler_Handle_Success(t *testing.T) {
	traces := createCriticalPathTestTrace()
	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{traces},
	}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	}

	_, output, err := handler.handle(context.Background(), nil, input)
	require.NoError(t, err)

	assert.Equal(t, "00000000000000000000000000000001", output.TraceID)
	assert.Positive(t, output.TotalDurationUs)
	assert.Positive(t, output.CriticalPathDurationUs)
	assert.NotEmpty(t, output.Segments)

	// Verify path contains span information
	for _, span := range output.Segments {
		assert.NotEmpty(t, span.SpanID)
		assert.NotEmpty(t, span.Service)
		assert.NotEmpty(t, span.SpanName)
	}
}

func TestGetCriticalPathHandler_Handle_EmptyTraceID(t *testing.T) {
	mockQS := &mockGetCriticalPathQueryService{}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "",
	}

	_, _, err := handler.handle(context.Background(), nil, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetCriticalPathHandler_Handle_InvalidTraceID(t *testing.T) {
	mockQS := &mockGetCriticalPathQueryService{}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "invalid-trace-id",
	}

	_, _, err := handler.handle(context.Background(), nil, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetCriticalPathHandler_Handle_QueryServiceError(t *testing.T) {
	mockQS := &mockGetCriticalPathQueryService{
		err: errors.New("query service error"),
	}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	}

	_, _, err := handler.handle(context.Background(), nil, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace")
}

func TestGetCriticalPathHandler_Handle_TraceNotFound(t *testing.T) {
	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{}, // empty traces
	}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	}

	_, _, err := handler.handle(context.Background(), nil, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetCriticalPathHandler_Handle_InvalidTrace(t *testing.T) {
	// Create a trace with no root span (all spans have parents)
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()

	span := ss.Spans().AppendEmpty()
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	span.SetParentSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 99}) // parent not in trace
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	span.SetStartTimestamp(pcommon.Timestamp(1000))
	span.SetEndTimestamp(pcommon.Timestamp(2000))

	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{traces},
	}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	}

	_, _, err := handler.handle(context.Background(), nil, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to compute critical path")
}

func TestGetCriticalPathHandler_Handle_MultipleServices(t *testing.T) {
	traces := ptrace.NewTraces()

	// First resource with service A
	rs1 := traces.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "service-a")
	ss1 := rs1.ScopeSpans().AppendEmpty()

	rootSpan := ss1.Spans().AppendEmpty()
	rootSpan.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	rootSpan.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	rootSpan.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	rootSpan.SetEndTimestamp(pcommon.Timestamp(101000 * 1000))
	rootSpan.SetName("operation-a")

	// Second resource with service B
	rs2 := traces.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "service-b")
	ss2 := rs2.ScopeSpans().AppendEmpty()

	childSpan := ss2.Spans().AppendEmpty()
	childSpan.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	childSpan.SetParentSpanID(rootSpan.SpanID())
	childSpan.SetTraceID(rootSpan.TraceID())
	childSpan.SetStartTimestamp(pcommon.Timestamp(20000 * 1000))
	childSpan.SetEndTimestamp(pcommon.Timestamp(40000 * 1000))
	childSpan.SetName("operation-b")

	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{traces},
	}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	}

	_, output, err := handler.handle(context.Background(), nil, input)
	require.NoError(t, err)

	// Verify multiple services are captured
	services := make(map[string]bool)
	for _, span := range output.Segments {
		services[span.Service] = true
	}
	assert.NotEmpty(t, services, "should have service names")
}

func TestGetCriticalPathHandler_Handle_UnknownService(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	// Don't set service.name attribute
	ss := rs.ScopeSpans().AppendEmpty()

	rootSpan := ss.Spans().AppendEmpty()
	rootSpan.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	rootSpan.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	rootSpan.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	rootSpan.SetEndTimestamp(pcommon.Timestamp(2000 * 1000))
	rootSpan.SetName("operation")

	mockQS := &mockGetCriticalPathQueryService{
		traces: []ptrace.Traces{traces},
	}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}

	input := types.GetCriticalPathInput{
		TraceID: "00000000000000000000000000000001",
	}

	_, output, err := handler.handle(context.Background(), nil, input)
	require.NoError(t, err)

	// Verify unknown service is used as fallback
	for _, span := range output.Segments {
		assert.Equal(t, "unknown", span.Service)
	}
}

func TestGetCriticalPathHandler_BuildOutput_MissingSpan(t *testing.T) {
	// Test buildOutput method directly to cover the case where a critical path section
	// refers to a span ID that is not in the trace span map (get_critical_path.go line 159).

	handler := &getCriticalPathHandler{}

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetSpanID([8]byte{1})
	span.SetTraceID([16]byte{1})
	span.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	span.SetEndTimestamp(pcommon.Timestamp(2000 * 1000))
	span.SetName("existing-span")

	traceID := span.TraceID().String()

	// Two sections: one for existing span, one for missing span
	sections := []criticalpath.Section{
		{
			SpanID:       span.SpanID().String(),
			SectionStart: 1000,
			SectionEnd:   2000,
		},
		{
			SpanID:       "missing-span-id", // This should be skipped
			SectionStart: 3000,
			SectionEnd:   4000,
		},
	}

	output := handler.buildOutput(traceID, traces, sections)

	// Should only have 1 segment for the existing span
	assert.Len(t, output.Segments, 1)
	assert.Equal(t, span.SpanID().String(), output.Segments[0].SpanID)
}
