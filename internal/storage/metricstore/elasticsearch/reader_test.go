// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

func tracerProvider(t *testing.T) (trace.TracerProvider, *tracetest.InMemoryExporter, func()) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	closer := func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	}
	return tp, exporter, closer
}

func TestGetLatencies(t *testing.T) {
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()

	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	reader, err := NewMetricsReader(config.Configuration{
		Servers: []string{"http://" + listener.Addr().String()},
	}, logger, tracer)
	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricstore.LatenciesQueryParameters{}
	r, err := reader.GetLatencies(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetCallRates(t *testing.T) {
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()
	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	reader, err := NewMetricsReader(config.Configuration{
		Servers: []string{"http://" + listener.Addr().String()},
	}, logger, tracer)

	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricstore.CallRateQueryParameters{}
	r, err := reader.GetCallRates(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetErrorRates(t *testing.T) {
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()

	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	reader, err := NewMetricsReader(config.Configuration{
		Servers: []string{"http://" + listener.Addr().String()},
	}, logger, tracer)

	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricstore.ErrorRateQueryParameters{}
	r, err := reader.GetErrorRates(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetMinStepDuration(t *testing.T) {
	params := metricstore.MinStepDurationQueryParameters{}
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()

	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	reader, err := NewMetricsReader(config.Configuration{
		Servers: []string{"http://" + listener.Addr().String()},
	}, logger, tracer)
	require.NoError(t, err)
	require.NotNil(t, reader)

	minStep, err := reader.GetMinStepDuration(context.Background(), &params)
	require.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}
