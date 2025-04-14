// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	reader := NewTraceReader(spanstore.SpanReaderParams{
		Logger: zap.NewNop(),
	})
	assert.Panics(t, func() {
		reader.GetTraces(context.Background(), v2api.GetTraceParams{})
	})
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
	reader := NewTraceReader(spanstore.SpanReaderParams{
		Logger: zap.NewNop(),
	})
	assert.Panics(t, func() {
		reader.FindTraceIDs(context.Background(), v2api.TraceQueryParams{})
	})
}
