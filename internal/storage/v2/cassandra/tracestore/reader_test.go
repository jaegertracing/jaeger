// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	casmocks "github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestNewTraceReader(t *testing.T) {
	session := getSessionWithError(nil)
	metricsFactory := metricstest.NewFactory(0)
	logger := zaptest.NewLogger(t)
	reader, err := NewTraceReader(session, metricsFactory, logger, telemetry.NoopSettings().TracerProvider.Tracer("testing"))
	require.NoError(t, err)
	traceids := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{})
	assert.NotNil(t, traceids)
	trace := reader.GetTraces(context.Background(), tracestore.GetTraceParams{})
	assert.NotNil(t, trace)
	traces := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{})
	assert.NotNil(t, traces)
}

func TestNewTraceReader_Error(t *testing.T) {
	session := getSessionWithError(errors.New("test error"))
	metricsFactory := metricstest.NewFactory(0)
	logger := zaptest.NewLogger(t)
	reader, err := NewTraceReader(session, metricsFactory, logger, telemetry.NoopSettings().TracerProvider.Tracer("test"))
	require.ErrorContains(t, err, "neither table operation_names_v2 nor operation_names exist")
	assert.Nil(t, reader)
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
	coreReader.On("GetTrace", mock.Anything, mock.Anything).Return(dbTrace, nil)
	traces := reader.GetTraces(context.Background(), tracestore.GetTraceParams{})
	for td, err := range traces {
		require.NoError(t, err)
		assert.Len(t, td, 1)
		testTraces(t, tracesStr, td[0])
		break
	}
}

func TestTraceReader_GetTraces_Error(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	coreReader.On("GetTrace", mock.Anything, mock.Anything).Return(dbmodel.Trace{}, errors.New("error"))
	for traces, err := range reader.GetTraces(context.Background(), tracestore.GetTraceParams{}) {
		require.ErrorContains(t, err, "error")
		require.Nil(t, traces)
	}
}

func TestTraceReader_FindTraces(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	tracesStr, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	dbTrace := dbmodel.Trace{Spans: []dbmodel.Span{span}}
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

func TestTraceReader_FindTraces_Error(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	coreReader.On("FindTraces", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
	for traces, err := range reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	}) {
		require.ErrorContains(t, err, "error")
		require.Nil(t, traces)
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

func TestTraceReader_FindTraceIDs_Error(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{reader: coreReader}
	coreReader.On("FindTraceIDs", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
	for traceIds, err := range reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	}) {
		require.ErrorContains(t, err, "error")
		require.Nil(t, traceIds)
	}
}

func getSessionWithError(err error) *casmocks.Session {
	tableCheckStmt := "SELECT * from %s limit 1"
	session := &casmocks.Session{}
	query := &casmocks.Query{}
	query.On("Exec").Return(err)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, "operation_names"),
		mock.Anything).Return(query)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, "operation_names_v2"),
		mock.Anything).Return(query)
	return session
}
