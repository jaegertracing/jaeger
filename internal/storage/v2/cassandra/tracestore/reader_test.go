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

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

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
