// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
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

func TestGetCriticalPathHandler_Handle_DuplicateSpanIDs(t *testing.T) {
	// The last span reuses the ID of its grandparent 0a.
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()
	for _, s := range []struct {
		id, parent byte
		startUs    uint64
		endUs      uint64
	}{
		{id: 0x01, startUs: 100, endUs: 200},
		{id: 0x0a, parent: 0x01, startUs: 110, endUs: 190},
		{id: 0x0b, parent: 0x0a, startUs: 120, endUs: 180},
		{id: 0x0a, parent: 0x0b, startUs: 130, endUs: 170},
	} {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID([16]byte{15: 1})
		span.SetSpanID([8]byte{7: s.id})
		span.SetParentSpanID([8]byte{7: s.parent})
		span.SetStartTimestamp(pcommon.Timestamp(s.startUs * 1000))
		span.SetEndTimestamp(pcommon.Timestamp(s.endUs * 1000))
	}

	handler := &getCriticalPathHandler{
		queryService: &mockGetCriticalPathQueryService{traces: []ptrace.Traces{traces}},
	}

	input := types.GetCriticalPathInput{TraceID: "00000000000000000000000000000001"}
	_, output, err := handler.handle(context.Background(), nil, input)
	require.NoError(t, err)

	spanIDs := make([]string, 0, len(output.Segments))
	for _, segment := range output.Segments {
		spanIDs = append(spanIDs, segment.SpanID)
	}
	assert.Equal(t, []string{
		"0000000000000001",
		"000000000000000a",
		"000000000000000b",
		"000000000000000a",
		"0000000000000001",
	}, spanIDs)
	assert.Equal(t, uint64(100), output.CriticalPathDurationUs)
}

func TestGetCriticalPathHandler_Handle_DuplicateSpanIDsReportFirstSpan(t *testing.T) {
	// Span 0a appears in two resources. The critical path is computed from the
	// first one, so the segment must also report the first span's name and service.
	type testSpan struct {
		id, parent byte
		startUs    uint64
		endUs      uint64
		name       string
	}
	traces := ptrace.NewTraces()
	for _, r := range []struct {
		service string
		spans   []testSpan
	}{
		{
			service: "svc-first",
			spans: []testSpan{
				{id: 0x01, startUs: 100, endUs: 200, name: "root-op"},
				{id: 0x0a, parent: 0x01, startUs: 110, endUs: 190, name: "first-op"},
			},
		},
		{
			service: "svc-second",
			spans: []testSpan{
				{id: 0x0a, parent: 0x01, startUs: 150, endUs: 160, name: "second-op"},
			},
		},
	} {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", r.service)
		ss := rs.ScopeSpans().AppendEmpty()
		for _, s := range r.spans {
			span := ss.Spans().AppendEmpty()
			span.SetTraceID([16]byte{15: 1})
			span.SetSpanID([8]byte{7: s.id})
			span.SetParentSpanID([8]byte{7: s.parent})
			span.SetStartTimestamp(pcommon.Timestamp(s.startUs * 1000))
			span.SetEndTimestamp(pcommon.Timestamp(s.endUs * 1000))
			span.SetName(s.name)
		}
	}

	handler := &getCriticalPathHandler{
		queryService: &mockGetCriticalPathQueryService{traces: []ptrace.Traces{traces}},
	}

	input := types.GetCriticalPathInput{TraceID: "00000000000000000000000000000001"}
	_, output, err := handler.handle(context.Background(), nil, input)
	require.NoError(t, err)

	var found bool
	for _, segment := range output.Segments {
		if segment.SpanID != "000000000000000a" {
			continue
		}
		found = true
		assert.Equal(t, "svc-first", segment.Service)
		assert.Equal(t, "first-op", segment.SpanName)
		assert.Equal(t, uint64(80), segment.SelfTimeUs)
	}
	assert.True(t, found, "critical path should include span 000000000000000a")
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

func TestGetCriticalPathHandler_BuildOutput_SegmentsCappedByLargestSelfTime(t *testing.T) {
	// buildOutput must cap the returned segments to the configured limit while
	// TotalSegmentCount still reflects every segment on the full critical path,
	// and critical_path_duration_us (summed before capping, in the handle method)
	// is unaffected by which segments get truncated here.
	handler := &getCriticalPathHandler{maxSpanDetailsPerRequest: 2}

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()

	small := ss.Spans().AppendEmpty()
	small.SetSpanID([8]byte{1})
	small.SetTraceID([16]byte{1})
	small.SetName("small")
	small.SetStartTimestamp(pcommon.Timestamp(0))
	small.SetEndTimestamp(pcommon.Timestamp(1000))

	medium := ss.Spans().AppendEmpty()
	medium.SetSpanID([8]byte{2})
	medium.SetTraceID([16]byte{1})
	medium.SetName("medium")
	medium.SetStartTimestamp(pcommon.Timestamp(0))
	medium.SetEndTimestamp(pcommon.Timestamp(1000))

	large := ss.Spans().AppendEmpty()
	large.SetSpanID([8]byte{3})
	large.SetTraceID([16]byte{1})
	large.SetName("large")
	large.SetStartTimestamp(pcommon.Timestamp(0))
	large.SetEndTimestamp(pcommon.Timestamp(1000))

	traceID := small.TraceID().String()

	// Three sections with distinct self times: 10us, 30us, 20us.
	sections := []criticalpath.Section{
		{SpanID: small.SpanID().String(), SectionStart: 0, SectionEnd: 10},
		{SpanID: medium.SpanID().String(), SectionStart: 10, SectionEnd: 40}, // self time 30
		{SpanID: large.SpanID().String(), SectionStart: 40, SectionEnd: 60},  // self time 20
	}

	output := handler.buildOutput(traceID, traces, sections)

	assert.Equal(t, 3, output.TotalSegmentCount, "total should reflect every computed segment")
	require.Len(t, output.Segments, 2, "segments should be capped to the configured limit")

	// The two largest self times (30, 20) must be kept; the smallest (10) dropped,
	// and the kept segments returned with the largest self time first.
	require.Equal(t, uint64(30), output.Segments[0].SelfTimeUs)
	require.Equal(t, uint64(20), output.Segments[1].SelfTimeUs)
}

func TestGetCriticalPathHandler_BuildOutput_SegmentsNotCappedWhenUnderLimit(t *testing.T) {
	// A limit of 0 means unlimited, matching the convention used by the other MCP tools.
	handler := &getCriticalPathHandler{maxSpanDetailsPerRequest: 0}

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetSpanID([8]byte{1})
	span.SetTraceID([16]byte{1})
	span.SetName("only-span")
	span.SetStartTimestamp(pcommon.Timestamp(0))
	span.SetEndTimestamp(pcommon.Timestamp(1000))

	traceID := span.TraceID().String()
	sections := []criticalpath.Section{
		{SpanID: span.SpanID().String(), SectionStart: 0, SectionEnd: 10},
	}

	output := handler.buildOutput(traceID, traces, sections)

	assert.Equal(t, 1, output.TotalSegmentCount)
	assert.Len(t, output.Segments, 1)
}
