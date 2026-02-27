// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var traceId [16]byte = [16]byte{1}

func TestNewTraceReader(t *testing.T) {
	reader := NewTraceReader(&mocks.CoreSpanReader{})
	assert.NotNil(t, reader)
	traceids := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{})
	assert.NotNil(t, traceids)
	trace := reader.GetTraces(context.Background(), tracestore.GetTraceParams{})
	assert.NotNil(t, trace)
	traces := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{})
	assert.NotNil(t, traces)
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
	reader.On("GetOperationsV2", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
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
	reader.On("GetOperationsV2", mock.Anything, mock.Anything).Return(expected, nil)
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetOperations(context.Background(), tracestore.OperationQueryParams{
		ServiceName: "service-1",
		SpanKind:    "some kind",
	})
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestTraceReader_GetTraces(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	tracesStr, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	dbTrace := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	span.TraceID = traceId
	coreReader.On("GetTrace", mock.Anything, mock.Anything).Return(dbTrace, nil)
	traces := reader.GetTraces(context.Background(), tracestore.GetTraceParams{})
	for td, err := range traces {
		require.NoError(t, err)
		assert.Len(t, td, 1)
		testTraces(t, tracesStr, td[0])
		break
	}
}

func TestTraceReader_FindTraces(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	tracesStr, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	dbTrace := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	span.TraceID = traceId
	dbTrace2 := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	coreReader.On("FindTraces", mock.Anything, mock.Anything).Return([]dbmodel.Trace{dbTrace, dbTrace2}, nil)
	traces := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	for td, err := range traces {
		require.NoError(t, err)
		assert.Len(t, td, 1)
		testTraces(t, tracesStr, td[0])
		break
	}
}

func TestTraceReader_FindTraceIDs(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	dbTraceIDs := []dbmodel.TraceID{
		[16]byte{1},
		[16]byte{2},
		[16]byte{3},
	}
	expected := make([]tracestore.FoundTraceID, 0, len(dbTraceIDs))
	for _, dbTraceID := range dbTraceIDs {
		expected = append(expected, tracestore.FoundTraceID{
			TraceID: pcommon.TraceID(dbTraceID),
		})
	}
	coreReader.On("FindTraceIDs", mock.Anything, mock.Anything).Return(dbTraceIDs, nil)
	for traceIds, err := range reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	}) {
		require.NoError(t, err)
		require.Equal(t, expected, traceIds)
	}
}
