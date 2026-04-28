// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	cassdbmodel "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestNewTraceReader(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	tracereader := NewTraceReader(&reader)
	require.NotNil(t, tracereader)
}

func TestGetServices(t *testing.T) {
	services := []string{"service-1", "service-2"}
	reader := mocks.CoreSpanReader{}
	reader.On("GetServices", mock.Anything).Return(services, nil)
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetServices(context.Background())
	require.NoError(t, err)
	require.Equal(t, services, got)
}

func TestGetOperationsErr(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	reader.On("GetOperations", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetOperations(context.Background(), tracestore.OperationQueryParams{
		ServiceName: "service-1",
		SpanKind:    "some kind",
	})
	require.ErrorContains(t, err, "error")
	require.Nil(t, got)
}

func TestGetOperations(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	expected := []tracestore.Operation{
		{
			Name:     "operation-1",
			SpanKind: "some kind",
		},
		{
			Name:     "operation-2",
			SpanKind: "some kind",
		},
	}
	reader.On("GetOperations", mock.Anything, mock.Anything).Return(expected, nil)
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetOperations(context.Background(), tracestore.OperationQueryParams{
		ServiceName: "service-1",
		SpanKind:    "some kind",
	})
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestGetTraces(t *testing.T) {
	traceID := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	spans := []cassdbmodel.Span{{TraceID: traceID, OperationName: "op"}}
	reader := mocks.CoreSpanReader{}
	reader.On("GetTrace", mock.Anything, traceID).Return(spans, nil)
	tracereader := &TraceReader{reader: &reader}
	var results []ptrace.Traces
	for batch, err := range tracereader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: pcommon.TraceID(traceID),
	}) {
		require.NoError(t, err)
		results = append(results, batch...)
	}
	require.Len(t, results, 1)
}

func TestGetTraces_NotFound(t *testing.T) {
	traceID := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	reader := mocks.CoreSpanReader{}
	reader.On("GetTrace", mock.Anything, traceID).Return([]cassdbmodel.Span(nil), nil)
	tracereader := &TraceReader{reader: &reader}
	var count int
	for _, err := range tracereader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: pcommon.TraceID(traceID),
	}) {
		require.NoError(t, err)
		count++
	}
	require.Equal(t, 0, count)
}

func TestGetTraces_Error(t *testing.T) {
	traceID := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	reader := mocks.CoreSpanReader{}
	reader.On("GetTrace", mock.Anything, traceID).Return([]cassdbmodel.Span(nil), errors.New("storage error"))
	tracereader := &TraceReader{reader: &reader}
	for _, err := range tracereader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: pcommon.TraceID(traceID),
	}) {
		require.ErrorContains(t, err, "storage error")
	}
}

func newTraceQueryParams(t *testing.T) tracestore.TraceQueryParams {
	t.Helper()
	attrs := pcommon.NewMap()
	return tracestore.TraceQueryParams{
		ServiceName:  "service-1",
		StartTimeMin: time.Now().Add(-time.Hour),
		StartTimeMax: time.Now(),
		Attributes:   attrs,
	}
}

func TestGetTraces_StopIteration(t *testing.T) {
	traceID1 := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	traceID2 := cassdbmodel.TraceID{1, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	spans := []cassdbmodel.Span{{TraceID: traceID1, OperationName: "op"}}
	reader := mocks.CoreSpanReader{}
	reader.On("GetTrace", mock.Anything, traceID1).Return(spans, nil)
	tracereader := &TraceReader{reader: &reader}
	var count int
	for range tracereader.GetTraces(context.Background(),
		tracestore.GetTraceParams{TraceID: pcommon.TraceID(traceID1)},
		tracestore.GetTraceParams{TraceID: pcommon.TraceID(traceID2)},
	) {
		count++
		break
	}
	require.Equal(t, 1, count)
}

func TestFindTraces(t *testing.T) {
	traceID := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	dbTraces := []cassdbmodel.Trace{
		{Spans: []cassdbmodel.Span{{TraceID: traceID, OperationName: "op"}}},
	}
	reader := mocks.CoreSpanReader{}
	reader.On("FindTraces", mock.Anything, mock.Anything).Return(mockIter(dbTraces, nil))
	tracereader := &TraceReader{reader: &reader}
	var results []ptrace.Traces
	for batch, err := range tracereader.FindTraces(context.Background(), newTraceQueryParams(t)) {
		require.NoError(t, err)
		results = append(results, batch...)
	}
	require.Len(t, results, 1)
}

func TestFindTraces_StopIteration(t *testing.T) {
	traceID := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	dbTraces := []cassdbmodel.Trace{
		{Spans: []cassdbmodel.Span{{TraceID: traceID, OperationName: "op1"}}},
		{Spans: []cassdbmodel.Span{{TraceID: traceID, OperationName: "op2"}}},
	}
	reader := mocks.CoreSpanReader{}
	reader.On("FindTraces", mock.Anything, mock.Anything).Return(mockIter(dbTraces, nil))
	tracereader := &TraceReader{reader: &reader}
	var count int
	for range tracereader.FindTraces(context.Background(), newTraceQueryParams(t)) {
		count++
		break
	}
	require.Equal(t, 1, count)
}

func TestFindTraces_Error(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	reader.On("FindTraces", mock.Anything, mock.Anything).Return(mockIter([]cassdbmodel.Trace{}, errors.New("find error")))
	tracereader := &TraceReader{reader: &reader}
	for _, err := range tracereader.FindTraces(context.Background(), newTraceQueryParams(t)) {
		require.ErrorContains(t, err, "find error")
	}
}

func TestFindTraceIDs(t *testing.T) {
	traceID := cassdbmodel.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	reader := mocks.CoreSpanReader{}
	reader.On("FindTraceIDs", mock.Anything, mock.Anything).Return([]cassdbmodel.TraceID{traceID}, nil)
	tracereader := &TraceReader{reader: &reader}
	var results []tracestore.FoundTraceID
	for batch, err := range tracereader.FindTraceIDs(context.Background(), newTraceQueryParams(t)) {
		require.NoError(t, err)
		results = append(results, batch...)
	}
	require.Len(t, results, 1)
	assert.Equal(t, pcommon.TraceID(traceID), results[0].TraceID)
}

func TestFindTraceIDs_Empty(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	reader.On("FindTraceIDs", mock.Anything, mock.Anything).Return([]cassdbmodel.TraceID{}, nil)
	tracereader := &TraceReader{reader: &reader}
	iterations := 0
	for _, err := range tracereader.FindTraceIDs(context.Background(), newTraceQueryParams(t)) {
		require.NoError(t, err)
		iterations++
	}
	require.Zero(t, iterations)
}

func TestFindTraceIDs_Error(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	reader.On("FindTraceIDs", mock.Anything, mock.Anything).Return([]cassdbmodel.TraceID(nil), errors.New("find error"))
	tracereader := &TraceReader{reader: &reader}
	for _, err := range tracereader.FindTraceIDs(context.Background(), newTraceQueryParams(t)) {
		require.ErrorContains(t, err, "find error")
	}
}

func mockIter(traces []cassdbmodel.Trace, err error) iter.Seq2[cassdbmodel.Trace, error] {
	return func(yield func(cassdbmodel.Trace, error) bool) {
		if err != nil {
			yield(cassdbmodel.Trace{}, err)
			return
		}
		for _, trace := range traces {
			if !yield(trace, err) {
				return
			}
		}
	}
}
