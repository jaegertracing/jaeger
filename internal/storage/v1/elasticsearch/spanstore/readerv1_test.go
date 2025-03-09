// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/mocks"
)

func withSpanReaderV1(fn func(r *SpanReaderV1, m *mocks.CoreSpanReader)) {
	spanReader := &mocks.CoreSpanReader{}
	r := &SpanReaderV1{
		spanReader: spanReader,
	}
	fn(r, spanReader)
}

func getTestingTrace(serviceName string) *model.Trace {
	return &model.Trace{Spans: []*model.Span{{
		Process: &model.Process{
			ServiceName: serviceName,
		},
	}}}
}

func TestSpanReaderV1_GetTrace(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		trace := getTestingTrace("service-1")
		m.On("GetTrace", mock.Anything, mock.AnythingOfType("spanstore.GetTraceParameters")).Return(trace, nil)
		actual, err := r.GetTrace(context.Background(), spanstore.GetTraceParameters{})
		require.NoError(t, err)
		assert.Equal(t, trace, actual)
	})
}

func TestSpanReaderV1_FindTraces(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		trace1 := getTestingTrace("service-1")
		trace2 := getTestingTrace("service-2")
		traces := []*model.Trace{trace1, trace2}
		m.On("FindTraces", mock.Anything, mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return(traces, nil)
		actual, err := r.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		require.NoError(t, err)
		assert.Equal(t, traces, actual)
	})
}

func TestSpanReaderV1_FindTraceIDs(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		traceId1Model := model.NewTraceID(0, 1)
		traceId2Model := model.NewTraceID(0, 2)
		traceId1 := dbmodel.TraceID(traceId1Model.String())
		traceId2 := dbmodel.TraceID(traceId2Model.String())
		traceIds := []dbmodel.TraceID{traceId1, traceId2}
		m.On("FindTraceIDs", mock.Anything, mock.AnythingOfType("*dbmodel.TraceQueryParameters")).Return(traceIds, nil)
		actual, err := r.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{})
		require.NoError(t, err)
		expected := []model.TraceID{traceId1Model, traceId2Model}
		assert.Equal(t, expected, actual)
	})
}

func TestSpanReaderV1_FindTraceIDs_Errors(t *testing.T) {
	tests := []struct {
		name              string
		returningTraceIDs []dbmodel.TraceID
		returningErr      error
		expectedError     string
	}{
		{
			name:          "error from core span reader",
			returningErr:  errors.New("error from core span reader"),
			expectedError: "error from core span reader",
		},
		{
			name:              "error from conversion",
			returningTraceIDs: []dbmodel.TraceID{dbmodel.TraceID("wrong-id")},
			expectedError:     "invalid syntax",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
				m.On("FindTraceIDs", mock.Anything, mock.AnythingOfType("*dbmodel.TraceQueryParameters")).Return(tt.returningTraceIDs, tt.returningErr)
				actual, err := r.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{})
				assert.Nil(t, actual)
				assert.ErrorContains(t, err, tt.expectedError)
			})
		})
	}
}

func TestSpanReaderV1_GetServices(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		services := []string{"service-1", "service-2"}
		m.On("GetServices", mock.Anything).Return(services, nil)
		actual, err := r.GetServices(context.Background())
		require.NoError(t, err)
		assert.Equal(t, services, actual)
	})
}

func TestSpanReaderV1_GetOperations(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		operation := []dbmodel.Operation{{Name: "operation-1", SpanKind: "kind-1"}}
		input := dbmodel.OperationQueryParameters{ServiceName: "service", SpanKind: "kind-1"}
		m.On("GetOperations", mock.Anything, input).Return(operation, nil)
		actual, err := r.GetOperations(context.Background(), spanstore.OperationQueryParameters{ServiceName: "service", SpanKind: "kind-1"})
		require.NoError(t, err)
		assert.Len(t, actual, 1)
		assert.Equal(t, operation[0].Name, actual[0].Name)
		assert.Equal(t, operation[0].SpanKind, actual[0].SpanKind)
	})
}

func TestSpanReaderV1_GetOperations_Error(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		input := dbmodel.OperationQueryParameters{ServiceName: "service", SpanKind: "kind-1"}
		m.On("GetOperations", mock.Anything, input).Return(nil, errors.New("error"))
		actual, err := r.GetOperations(context.Background(), spanstore.OperationQueryParameters{ServiceName: "service", SpanKind: "kind-1"})
		require.Error(t, err, "error")
		assert.Nil(t, actual)
	})
}
