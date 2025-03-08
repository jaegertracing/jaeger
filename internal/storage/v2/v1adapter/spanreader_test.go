// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func TestSpanReader_GetTrace(t *testing.T) {
	tests := []struct {
		name          string
		query         spanstore.GetTraceParameters
		expectedQuery tracestore.GetTraceParams
		traces        []ptrace.Traces
		expectedTrace *model.Trace
		err           error
		expectedErr   error
	}{
		{
			name: "error getting trace",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name: "empty traces",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			traces:      []ptrace.Traces{},
			expectedErr: spanstore.ErrTraceNotFound,
		},
		{
			name: "too many traces found",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			traces: func() []ptrace.Traces {
				traces1 := ptrace.NewTraces()
				resources1 := traces1.ResourceSpans().AppendEmpty()
				resources1.Resource().Attributes().PutStr("service.name", "service1")
				scopes1 := resources1.ScopeSpans().AppendEmpty()
				span1 := scopes1.Spans().AppendEmpty()
				span1.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2})

				traces2 := ptrace.NewTraces()
				resources2 := traces2.ResourceSpans().AppendEmpty()
				resources2.Resource().Attributes().PutStr("service.name", "service1")
				scopes2 := resources2.ScopeSpans().AppendEmpty()
				span2 := scopes2.Spans().AppendEmpty()
				span2.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3})

				return []ptrace.Traces{traces1, traces2}
			}(),
			expectedErr: errTooManyTracesFound,
		},
		{
			name: "success",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			traces: func() []ptrace.Traces {
				traces := ptrace.NewTraces()
				resources := traces.ResourceSpans().AppendEmpty()
				resources.Resource().Attributes().PutStr("service.name", "service")
				scopes := resources.ScopeSpans().AppendEmpty()
				span := scopes.Spans().AppendEmpty()
				span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2})
				span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3})
				span.SetName("span")
				span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 0).UTC()))
				return []ptrace.Traces{traces}
			}(),
			expectedTrace: &model.Trace{
				Spans: []*model.Span{
					{
						TraceID:       model.NewTraceID(1, 2),
						SpanID:        model.NewSpanID(3),
						OperationName: "span",
						References:    []model.SpanRef{},
						Tags:          make([]model.KeyValue, 0),
						Process:       model.NewProcess("service", make([]model.KeyValue, 0)),
						StartTime:     time.Unix(0, 0).UTC(),
					},
				},
			},
		},
	}
	for _, test := range tests {
		tr := tracestoremocks.Reader{}
		tr.On("GetTraces", mock.Anything, mock.Anything).
			Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield(test.traces, test.err)
			})).Once()

		sr := NewSpanReader(&tr)
		trace, err := sr.GetTrace(context.Background(), test.query)
		require.ErrorIs(t, err, test.expectedErr)
		require.Equal(t, test.expectedTrace, trace)
	}
}

func TestSpanReader_GetServices(t *testing.T) {
	tests := []struct {
		name             string
		services         []string
		expectedServices []string
		err              error
		expectedErr      error
	}{
		{
			name:        "error getting services",
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name:             "no services",
			services:         []string{},
			expectedServices: []string{},
		},
		{
			name:             "multiple services",
			services:         []string{"service1", "service2"},
			expectedServices: []string{"service1", "service2"},
		},
	}

	for _, test := range tests {
		tr := tracestoremocks.Reader{}
		tr.On("GetServices", mock.Anything).
			Return(test.services, test.err).Once()

		sr := NewSpanReader(&tr)
		services, err := sr.GetServices(context.Background())
		require.ErrorIs(t, err, test.expectedErr)
		require.Equal(t, test.expectedServices, services)
	}
}

func TestSpanReader_GetOperations(t *testing.T) {
	tests := []struct {
		name               string
		query              spanstore.OperationQueryParameters
		expectedQuery      tracestore.OperationQueryParams
		operations         []tracestore.Operation
		expectedOperations []spanstore.Operation
		err                error
		expectedErr        error
	}{
		{
			name: "error getting operations",
			query: spanstore.OperationQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.OperationQueryParams{
				ServiceName: "service1",
			},
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name: "no operations",
			query: spanstore.OperationQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.OperationQueryParams{
				ServiceName: "service1",
			},
			operations:         []tracestore.Operation{},
			expectedOperations: []spanstore.Operation{},
		},
		{
			name: "multiple operations",
			query: spanstore.OperationQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.OperationQueryParams{
				ServiceName: "service1",
			},
			operations: []tracestore.Operation{
				{Name: "operation1", SpanKind: "kind1"},
				{Name: "operation2", SpanKind: "kind2"},
			},
			expectedOperations: []spanstore.Operation{
				{Name: "operation1", SpanKind: "kind1"},
				{Name: "operation2", SpanKind: "kind2"},
			},
		},
	}

	for _, test := range tests {
		tr := tracestoremocks.Reader{}
		tr.On("GetOperations", mock.Anything, test.expectedQuery).
			Return(test.operations, test.err).Once()

		sr := NewSpanReader(&tr)
		ops, err := sr.GetOperations(context.Background(), test.query)
		require.ErrorIs(t, err, test.expectedErr)
		require.Equal(t, test.expectedOperations, ops)
	}
}

func TestSpanReader_FindTraces(t *testing.T) {
	tests := []struct {
		name           string
		query          *spanstore.TraceQueryParameters
		expectedQuery  tracestore.TraceQueryParams
		traces         []ptrace.Traces
		expectedTraces []*model.Trace
		err            error
		expectedErr    error
	}{
		{
			name: "error finding traces",
			query: &spanstore.TraceQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.TraceQueryParams{
				ServiceName: "service1",
				Attributes:  pcommon.NewMap(),
			},
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name: "no traces found",
			query: &spanstore.TraceQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.TraceQueryParams{
				ServiceName: "service1",
				Attributes:  pcommon.NewMap(),
			},
			traces:         []ptrace.Traces{},
			expectedTraces: nil,
		},
		{
			name: "multiple traces found",
			query: &spanstore.TraceQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.TraceQueryParams{
				ServiceName: "service1",
				Attributes:  pcommon.NewMap(),
			},
			traces: func() []ptrace.Traces {
				traces1 := ptrace.NewTraces()
				resources1 := traces1.ResourceSpans().AppendEmpty()
				resources1.Resource().Attributes().PutStr("service.name", "service1")
				scopes1 := resources1.ScopeSpans().AppendEmpty()
				span1 := scopes1.Spans().AppendEmpty()
				span1.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2})
				span1.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3})
				span1.SetName("span1")
				span1.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 0).UTC()))

				traces2 := ptrace.NewTraces()
				resources2 := traces2.ResourceSpans().AppendEmpty()
				resources2.Resource().Attributes().PutStr("service.name", "service1")
				scopes2 := resources2.ScopeSpans().AppendEmpty()
				span2 := scopes2.Spans().AppendEmpty()
				span2.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 5})
				span2.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 6})
				span2.SetName("span2")
				span2.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 0).UTC()))

				return []ptrace.Traces{traces1, traces2}
			}(),
			expectedTraces: []*model.Trace{
				{
					Spans: []*model.Span{
						{
							TraceID:       model.NewTraceID(1, 2),
							SpanID:        model.NewSpanID(3),
							OperationName: "span1",
							References:    make([]model.SpanRef, 0),
							Tags:          model.KeyValues{},
							Process:       model.NewProcess("service1", make([]model.KeyValue, 0)),
							StartTime:     time.Unix(0, 0).UTC(),
						},
					},
				},
				{
					Spans: []*model.Span{
						{
							TraceID:       model.NewTraceID(4, 5),
							SpanID:        model.NewSpanID(6),
							OperationName: "span2",
							References:    make([]model.SpanRef, 0),
							Tags:          model.KeyValues{},
							Process:       model.NewProcess("service1", make([]model.KeyValue, 0)),
							StartTime:     time.Unix(0, 0).UTC(),
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		tr := tracestoremocks.Reader{}
		tr.On("FindTraces", mock.Anything, test.expectedQuery).
			Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield(test.traces, test.err)
			})).Once()

		sr := NewSpanReader(&tr)
		traces, err := sr.FindTraces(context.Background(), test.query)
		require.ErrorIs(t, err, test.expectedErr)
		require.Equal(t, test.expectedTraces, traces)
	}
}

func TestSpanReader_FindTraceIDs(t *testing.T) {
	tests := []struct {
		name             string
		query            *spanstore.TraceQueryParameters
		expectedQuery    tracestore.TraceQueryParams
		traceIDs         []tracestore.FoundTraceID
		expectedTraceIDs []model.TraceID
		err              error
		expectedErr      error
	}{
		{
			name: "error finding trace IDs",
			query: &spanstore.TraceQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.TraceQueryParams{
				ServiceName: "service1",
				Attributes:  pcommon.NewMap(),
			},
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name: "no trace IDs found",
			query: &spanstore.TraceQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.TraceQueryParams{
				ServiceName: "service1",
				Attributes:  pcommon.NewMap(),
			},
			traceIDs:         []tracestore.FoundTraceID{},
			expectedTraceIDs: nil,
		},
		{
			name: "multiple trace IDs found",
			query: &spanstore.TraceQueryParameters{
				ServiceName: "service1",
			},
			expectedQuery: tracestore.TraceQueryParams{
				ServiceName: "service1",
				Attributes:  pcommon.NewMap(),
			},
			traceIDs: []tracestore.FoundTraceID{
				{
					TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 4},
				},
			},
			expectedTraceIDs: []model.TraceID{
				model.NewTraceID(1, 2),
				model.NewTraceID(3, 4),
			},
		},
	}

	for _, test := range tests {
		tr := tracestoremocks.Reader{}
		tr.On("FindTraceIDs", mock.Anything, test.expectedQuery).
			Return(iter.Seq2[[]tracestore.FoundTraceID, error](func(yield func([]tracestore.FoundTraceID, error) bool) {
				yield(test.traceIDs, test.err)
			})).Once()

		sr := NewSpanReader(&tr)
		traceIDs, err := sr.FindTraceIDs(context.Background(), test.query)
		require.ErrorIs(t, err, test.expectedErr)
		require.Equal(t, test.expectedTraceIDs, traceIDs)
	}
}
