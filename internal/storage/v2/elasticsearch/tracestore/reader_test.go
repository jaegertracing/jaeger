// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/mocks"
	v2api "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestTraceReader_GetServices(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{spanReader: coreReader}
	services := []string{"service1", "service2"}
	coreReader.On("GetServices", mock.Anything).Return(services, nil)
	actual, err := reader.GetServices(context.Background())
	require.NoError(t, err)
	require.Equal(t, services, actual)
}

func TestTraceReader_GetOperations(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{spanReader: coreReader}
	operations := []dbmodel.Operation{
		{
			Name:     "op-1",
			SpanKind: "kind--1",
		},
		{
			Name:     "op-2",
			SpanKind: "kind--2",
		},
	}
	coreReader.On("GetOperations", mock.Anything, mock.Anything).Return(operations, nil)
	expected := []v2api.Operation{
		{
			Name:     "op-1",
			SpanKind: "kind--1",
		},
		{
			Name:     "op-2",
			SpanKind: "kind--2",
		},
	}
	actual, err := reader.GetOperations(context.Background(), v2api.OperationQueryParams{})
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestTraceReader_GetOperations_Error(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{spanReader: coreReader}
	coreReader.On("GetOperations", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
	operations, err := reader.GetOperations(context.Background(), v2api.OperationQueryParams{})
	require.EqualError(t, err, "error")
	require.Nil(t, operations)
}

func TestTraceReader_GetTraces(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{spanReader: coreReader}
	tracesStr, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	coreReader.On("GetTraces", mock.Anything, mock.Anything).Return([]dbmodel.Trace{{Spans: []dbmodel.Span{span}}}, nil)
	traces := reader.GetTraces(context.Background(), v2api.GetTraceParams{})
	for td, err := range traces {
		require.NoError(t, err)
		assert.Len(t, td, 1)
		testTraces(t, tracesStr, td[0])
	}
}

func TestTraceReader_GetTraces_Errors(t *testing.T) {
	tests := []struct {
		name            string
		returningErr    error
		returningTraces []dbmodel.Trace
		expectedErr     string
	}{
		{
			name:         "some error from db GetTraces",
			returningErr: errors.New("some error"),
			expectedErr:  "some error",
		},
		{
			name: "conversion error",
			returningTraces: []dbmodel.Trace{
				{
					Spans: []dbmodel.Span{
						{
							TraceID: "wrong-trace-id",
						},
					},
				},
			},
			expectedErr: "encoding/hex: invalid byte: U+0077 'w'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreReader := &mocks.CoreSpanReader{}
			reader := TraceReader{spanReader: coreReader}
			coreReader.On("GetTraces", mock.Anything, mock.Anything).Return(tt.returningTraces, tt.returningErr)
			traces := reader.GetTraces(context.Background(), v2api.GetTraceParams{})
			for trace, err := range traces {
				require.Nil(t, trace)
				require.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}

func TestTraceReader_FindTraces(t *testing.T) {
	reader := NewTraceReader(spanstore.SpanReaderParams{
		Logger: zap.NewNop(),
	})
	assert.Panics(t, func() {
		reader.FindTraces(context.Background(), v2api.TraceQueryParams{})
	})
}

func TestTraceReader_FindTraceIDs(t *testing.T) {
	coreReader := &mocks.CoreSpanReader{}
	reader := TraceReader{spanReader: coreReader}
	dbTraceIDs := []dbmodel.TraceID{
		"00000000000000010000000000000000",
		"00000000000000020000000000000000",
		"00000000000000030000000000000000",
	}
	expected := make([]v2api.FoundTraceID, 0, len(dbTraceIDs))
	for _, dbTraceID := range dbTraceIDs {
		expected = append(expected, fromDBTraceId(t, dbTraceID))
	}
	coreReader.On("FindTraceIDs", mock.Anything, mock.Anything).Return(dbTraceIDs, nil)
	for traceIds, err := range reader.FindTraceIDs(context.Background(), v2api.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	}) {
		require.NoError(t, err)
		require.Equal(t, expected, traceIds)
	}
}

func TestTraceReader_FindTraceIDs_Error(t *testing.T) {
	tests := []struct {
		name                   string
		errFromCoreReader      error
		traceIdsFromCoreReader []dbmodel.TraceID
		expectedErr            string
	}{
		{
			name:              "some error from core reader",
			errFromCoreReader: errors.New("some error from core reader"),
			expectedErr:       "some error from core reader",
		},
		{
			name:                   "wrong trace id sent from core reader",
			traceIdsFromCoreReader: []dbmodel.TraceID{"wrong-id"},
			expectedErr:            "encoding/hex: invalid byte: U+0077 'w'",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coreReader := &mocks.CoreSpanReader{}
			attrs := pcommon.NewMap()
			attrs.PutStr("key1", "val1")
			ts := time.Now()
			traceQueryParams := v2api.TraceQueryParams{
				Attributes:    attrs,
				StartTimeMin:  ts,
				ServiceName:   "testing-service-name",
				OperationName: "testing-operation-name",
				StartTimeMax:  ts.Add(1 * time.Hour),
				DurationMin:   1 * time.Hour,
				DurationMax:   1 * time.Hour,
				SearchDepth:   10,
			}
			dbTraceQueryParams := dbmodel.TraceQueryParameters{
				Tags:          map[string]string{"key1": "val1"},
				StartTimeMin:  ts,
				ServiceName:   "testing-service-name",
				OperationName: "testing-operation-name",
				StartTimeMax:  ts.Add(1 * time.Hour),
				DurationMin:   1 * time.Hour,
				DurationMax:   1 * time.Hour,
				NumTraces:     10,
			}
			coreReader.On("FindTraceIDs", mock.Anything, dbTraceQueryParams).Return(test.traceIdsFromCoreReader, test.errFromCoreReader)
			reader := TraceReader{spanReader: coreReader}
			for traceIds, err := range reader.FindTraceIDs(context.Background(), traceQueryParams) {
				require.ErrorContains(t, err, test.expectedErr)
				require.Nil(t, traceIds)
			}
		})
	}
}

func fromDBTraceId(t *testing.T, traceID dbmodel.TraceID) v2api.FoundTraceID {
	traceId, err := fromDbTraceId(traceID)
	require.NoError(t, err)
	return v2api.FoundTraceID{
		TraceID: traceId,
	}
}
