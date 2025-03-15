// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"fmt"
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

func getTestingTrace(traceID model.TraceID, spanId model.SpanID) *dbmodel.Trace {
	return &dbmodel.Trace{Spans: []*dbmodel.Span{{
		TraceID: dbmodel.TraceID(traceID.String()),
		SpanID:  dbmodel.SpanID(spanId.String()),
	}}}
}

func TestSpanReaderV1_GetTrace(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		traceID1 := model.NewTraceID(0, 1)
		spanID1 := model.NewSpanID(1)
		trace := getTestingTrace(traceID1, spanID1)
		m.On("GetTrace", mock.Anything, mock.AnythingOfType("[]dbmodel.TraceID")).Return([]*dbmodel.Trace{trace}, nil)
		actual, err := r.GetTrace(context.Background(), spanstore.GetTraceParameters{})
		require.NoError(t, err)
		assert.Len(t, actual.Spans, 1)
		assert.Equal(t, traceID1, actual.Spans[0].TraceID)
	})
}

func TestSpanReaderV1_FindTraces(t *testing.T) {
	withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
		traceID1 := model.NewTraceID(0, 1)
		spanID1 := model.NewSpanID(1)
		traceID2 := model.NewTraceID(0, 2)
		spanID2 := model.NewSpanID(2)
		trace1 := getTestingTrace(traceID1, spanID1)
		trace2 := getTestingTrace(traceID2, spanID2)
		traces := []*dbmodel.Trace{trace1, trace2}
		m.On("FindTraces", mock.Anything, mock.AnythingOfType("*dbmodel.TraceQueryParameters")).Return(traces, nil)
		actual, err := r.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		require.NoError(t, err)
		assert.Len(t, actual, 2)
		assert.Len(t, actual[0].Spans, 1)
		assert.Len(t, actual[1].Spans, 1)
		assert.Equal(t, traceID1, actual[0].Spans[0].TraceID)
		assert.Equal(t, traceID2, actual[1].Spans[0].TraceID)
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
			expectedError:     "making traceID from string 'wrong-id' failed: strconv.ParseUint: parsing \"wrong-id\": invalid syntax",
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

func TestSpanReaderV1_ArchiveTraces(t *testing.T) {
	testCases := []struct {
		useAliases bool
		suffix     string
		expected   string
	}{
		{false, "", "jaeger-span-"},
		{true, "", "jaeger-span-read"},
		{false, "foobar", "jaeger-span-"},
		{true, "foobar", "jaeger-span-foobar"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("useAliases=%v suffix=%s", tc.useAliases, tc.suffix), func(t *testing.T) {
			withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
				m.On("GetTrace", mock.Anything, mock.AnythingOfType("[]dbmodel.TraceID")).Return([]*dbmodel.Trace{}, nil)
				query := spanstore.GetTraceParameters{}
				trace, err := r.GetTrace(context.Background(), query)
				require.Nil(t, trace)
				require.EqualError(t, err, "trace not found")
			})
		})
	}
}

type traceError struct {
	name            string
	returningErr    error
	expectedError   string
	returningTraces []*dbmodel.Trace
}

func getTraceErrTests() []traceError {
	return []traceError{
		{
			name:            "conversion error",
			expectedError:   "span conversion error, because lacks elements",
			returningTraces: []*dbmodel.Trace{getBadTrace()},
		},
		{
			name:          "generic error",
			returningErr:  errors.New("error"),
			expectedError: "error",
		},
		{
			name:            "trace not found",
			returningTraces: []*dbmodel.Trace{},
			expectedError:   "trace not found",
		},
	}
}

func TestSpanReaderV1_GetTraceError(t *testing.T) {
	tests := getTraceErrTests()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
				m.On("GetTrace", mock.Anything, mock.AnythingOfType("[]dbmodel.TraceID")).Return(tt.returningTraces, tt.returningErr)
				query := spanstore.GetTraceParameters{}
				trace, err := r.GetTrace(context.Background(), query)
				require.Error(t, err, tt.expectedError)
				require.Nil(t, trace)
			})
		})
	}
}

func TestSpanReaderV1_FindTracesError(t *testing.T) {
	tests := getTraceErrTests()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSpanReaderV1(func(r *SpanReaderV1, m *mocks.CoreSpanReader) {
				m.On("FindTraces", mock.Anything, mock.AnythingOfType("*dbmodel.TraceQueryParameters")).Return(tt.returningTraces, tt.returningErr)
				query := &spanstore.TraceQueryParameters{}
				trace, err := r.FindTraces(context.Background(), query)
				require.Error(t, err, tt.expectedError)
				require.Nil(t, trace)
			})
		})
	}
}

func getBadTrace() *dbmodel.Trace {
	return &dbmodel.Trace{
		Spans: []*dbmodel.Span{
			{
				OperationName: "testing-operation",
			},
		},
	}
}

func TestTraceIDsStringsToModelsConversion(t *testing.T) {
	traceIDs, err := toModelTraceIDs([]dbmodel.TraceID{"1", "2", "3"})
	require.NoError(t, err)
	assert.Len(t, traceIDs, 3)
	assert.Equal(t, model.NewTraceID(0, 1), traceIDs[0])

	traceIDs, err = toModelTraceIDs([]dbmodel.TraceID{"dsfjsdklfjdsofdfsdbfkgbgoaemlrksdfbsdofgerjl"})
	require.EqualError(t, err, "making traceID from string 'dsfjsdklfjdsofdfsdbfkgbgoaemlrksdfbsdofgerjl' failed: TraceID cannot be longer than 32 hex characters: dsfjsdklfjdsofdfsdbfkgbgoaemlrksdfbsdofgerjl")
	assert.Empty(t, traceIDs)
}

func TestConvertTraceIDsStringsToModels(t *testing.T) {
	ids, err := toModelTraceIDs([]dbmodel.TraceID{"1", "2", "01", "02", "001", "002"})
	require.NoError(t, err)
	assert.Equal(t, []model.TraceID{model.NewTraceID(0, 1), model.NewTraceID(0, 2)}, ids)
	_, err = toModelTraceIDs([]dbmodel.TraceID{"1", "2", "01", "02", "001", "002", "blah"})
	require.Error(t, err)
}
