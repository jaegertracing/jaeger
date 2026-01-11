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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
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
	mockQS := &mockGetCriticalPathQueryService{}
	handler := &getCriticalPathHandler{
		queryService: mockQS,
	}
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.queryService)
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
		assert.NotEmpty(t, span.Operation)
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
