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
		maxSegments:  20,
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
	assert.Equal(t, len(output.Segments), output.TotalSegmentCount)

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
		maxSegments:  20,
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
		maxSegments:  20,
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
		maxSegments:  20,
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
		maxSegments:  20,
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
		maxSegments:  20,
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
		maxSegments:  20,
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
		maxSegments:  20,
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

func TestGetCriticalPathHandler_BuildOutput_Truncation(t *testing.T) {
	handler := &getCriticalPathHandler{maxSegments: 2}

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()

	// Create 3 spans with different self times
	span1 := ss.Spans().AppendEmpty()
	span1.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	span1.SetTraceID([16]byte{1})
	span1.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	span1.SetEndTimestamp(pcommon.Timestamp(100100 * 1000)) // 100ms self time
	span1.SetName("span-100ms")

	span2 := ss.Spans().AppendEmpty()
	span2.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	span2.SetTraceID([16]byte{1})
	span2.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	span2.SetEndTimestamp(pcommon.Timestamp(10001 * 1000)) // ~10ms self time
	span2.SetName("span-10ms")

	span3 := ss.Spans().AppendEmpty()
	span3.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3})
	span3.SetTraceID([16]byte{1})
	span3.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	span3.SetEndTimestamp(pcommon.Timestamp(1002 * 1000)) // ~2ms self time
	span3.SetName("span-2ms")

	traceID := span1.TraceID().String()

	sections := []criticalpath.Section{
		{SpanID: span1.SpanID().String(), SectionStart: 1000, SectionEnd: 100100},
		{SpanID: span2.SpanID().String(), SectionStart: 1000, SectionEnd: 10001},
		{SpanID: span3.SpanID().String(), SectionStart: 1000, SectionEnd: 1002},
	}

	output := handler.buildOutput(traceID, traces, sections, types.GetCriticalPathInput{MaxSegments: 2})

	assert.Equal(t, 3, output.TotalSegmentCount, "total before truncation")
	assert.Len(t, output.Segments, 2, "should be truncated to 2")
	// The top 2 by self_time should be span1 (100ms) and span2 (10ms)
	assert.Equal(t, "span-100ms", output.Segments[0].SpanName, "first by self_time")
	assert.Equal(t, "span-10ms", output.Segments[1].SpanName, "second by self_time")
}

func TestGetCriticalPathHandler_BuildOutput_NoTruncation(t *testing.T) {
	handler := &getCriticalPathHandler{maxSegments: 20}

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()

	span := ss.Spans().AppendEmpty()
	span.SetSpanID([8]byte{1})
	span.SetTraceID([16]byte{1})
	span.SetStartTimestamp(pcommon.Timestamp(1000 * 1000))
	span.SetEndTimestamp(pcommon.Timestamp(2000 * 1000))
	span.SetName("single-span")

	sections := []criticalpath.Section{
		{SpanID: span.SpanID().String(), SectionStart: 1000, SectionEnd: 2000},
	}

	output := handler.buildOutput(span.TraceID().String(), traces, sections, types.GetCriticalPathInput{})

	assert.Equal(t, 1, output.TotalSegmentCount)
	assert.Len(t, output.Segments, 1)
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

	output := handler.buildOutput(traceID, traces, sections, types.GetCriticalPathInput{})

	// Should only have 1 segment for the existing span
	assert.Len(t, output.Segments, 1)
	assert.Equal(t, span.SpanID().String(), output.Segments[0].SpanID)
}
