// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracesvc

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// mockSpanReader implements spanstore.Reader for testing
type mockSpanReader struct {
	services   []string
	operations []spanstore.Operation
	traces     []*model.Trace
	err        error
}

func (m *mockSpanReader) GetServices(_ context.Context) ([]string, error) {
	return m.services, m.err
}

func (m *mockSpanReader) GetOperations(_ context.Context, _ spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	return m.operations, m.err
}

func (m *mockSpanReader) GetTrace(_ context.Context, _ spanstore.GetTraceParameters) (*model.Trace, error) {
	if len(m.traces) > 0 {
		return m.traces[0], m.err
	}
	return nil, m.err
}

func (m *mockSpanReader) FindTraces(_ context.Context, _ *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return m.traces, m.err
}

func (m *mockSpanReader) FindTraceIDs(_ context.Context, _ *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return nil, m.err
}

// mockTraceReader implements tracestore.Reader for testing
type mockTraceReader struct{}

func (m *mockTraceReader) GetTraces(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {}
}

func (m *mockTraceReader) GetServices(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockTraceReader) GetOperations(_ context.Context, _ tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return nil, nil
}

func (m *mockTraceReader) FindTraces(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {}
}

func (m *mockTraceReader) FindTraceIDs(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {}
}

// mockDepReader implements depstore.Reader for testing
type mockDepReader struct{}

func (m *mockDepReader) GetDependencies(_ context.Context, _ depstore.QueryParameters) ([]model.DependencyLink, error) {
	return nil, nil
}

func TestNewTraceService(t *testing.T) {
	mockSpanReader := &mockSpanReader{}
	mockTraceReader := &mockTraceReader{}
	mockDepReader := &mockDepReader{}

	svc := NewTraceService(mockSpanReader, mockTraceReader, mockDepReader)
	assert.NotNil(t, svc)
	assert.Equal(t, mockSpanReader, svc.v1SpanReader)
	assert.Equal(t, mockTraceReader, svc.v2TraceReader)
	assert.Equal(t, mockDepReader, svc.depReader)
}

func TestTraceService_GetServices(t *testing.T) {
	mockSpanReader := &mockSpanReader{
		services: []string{"service1", "service2"},
	}
	mockTraceReader := &mockTraceReader{}
	mockDepReader := &mockDepReader{}

	svc := NewTraceService(mockSpanReader, mockTraceReader, mockDepReader)
	services, err := svc.GetServices(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"service1", "service2"}, services)
}

func TestTraceService_GetOperations(t *testing.T) {
	expectedOps := []spanstore.Operation{
		{Name: "op1", SpanKind: "server"},
		{Name: "op2", SpanKind: "client"},
	}
	mockSpanReader := &mockSpanReader{
		operations: expectedOps,
	}
	mockTraceReader := &mockTraceReader{}
	mockDepReader := &mockDepReader{}

	svc := NewTraceService(mockSpanReader, mockTraceReader, mockDepReader)
	ops, err := svc.GetOperations(context.Background(), spanstore.OperationQueryParameters{
		ServiceName: "service1",
	})

	require.NoError(t, err)
	assert.Equal(t, expectedOps, ops)
}

func TestTraceService_FindTraces(t *testing.T) {
	expectedTraces := []*model.Trace{
		{
			Spans: []*model.Span{
				{
					TraceID:       model.TraceID{Low: 1},
					SpanID:        model.SpanID(1),
					OperationName: "test-op",
				},
			},
		},
	}
	mockSpanReader := &mockSpanReader{
		traces: expectedTraces,
	}
	mockTraceReader := &mockTraceReader{}
	mockDepReader := &mockDepReader{}

	svc := NewTraceService(mockSpanReader, mockTraceReader, mockDepReader)
	traces, err := svc.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})

	require.NoError(t, err)
	assert.Equal(t, expectedTraces, traces)
}

func TestTraceService_GetTrace(t *testing.T) {
	expectedTrace := &model.Trace{
		Spans: []*model.Span{
			{
				TraceID:       model.TraceID{Low: 1},
				SpanID:        model.SpanID(1),
				OperationName: "test-op",
			},
		},
	}
	mockSpanReader := &mockSpanReader{
		traces: []*model.Trace{expectedTrace},
	}
	mockTraceReader := &mockTraceReader{}
	mockDepReader := &mockDepReader{}

	svc := NewTraceService(mockSpanReader, mockTraceReader, mockDepReader)
	trace, err := svc.GetTrace(context.Background(), spanstore.GetTraceParameters{})

	require.NoError(t, err)
	assert.Equal(t, expectedTrace, trace)
}
