// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// mockServerResponse simulates a basic Elasticsearch version response.
var mockServerResponse = []byte(`
{
    "version": {
       "number": "6.8.0"
    }
}
`)

// tracerProvider creates a new OpenTelemetry TracerProvider for testing.
func tracerProvider(t *testing.T) (trace.TracerProvider, func()) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	closer := func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	}
	return tp, closer
}

func clientProvider(t *testing.T, c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, func()) {
	client, err := config.NewClient(c, logger, metricsFactory)
	require.NoError(t, err)
	require.NotNil(t, client)

	closer := func() {
		require.NoError(t, client.Close())
	}
	return client, closer
}

// setupMetricsReader provides a common setup for tests requiring a MetricsReader.
// It initializes a mock HTTP server and configures the reader to use it.
func setupMetricsReader(t *testing.T) (*MetricsReader, func()) {
	logger := zap.NewNop()
	tracer, closer := tracerProvider(t)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockServerResponse)
	}))
	t.Cleanup(mockServer.Close)
	cfg := config.Configuration{
		Servers:  []string{mockServer.URL},
		LogLevel: "debug",
	}

	client, clientCloser := clientProvider(t, &cfg, logger, telemetry.NoopSettings().Metrics)
	reader := NewMetricsReader(cfg, logger, tracer, client)

	require.NotNil(t, reader)

	// Return a cleanup function that combines the tracer shutdown and any other necessary cleanup.
	combinedCloser := func() {
		closer() // Shut down the tracer
		clientCloser()
		// Add any other specific cleanup if needed in the future
	}

	return reader, combinedCloser
}

func TestGetLatencies(t *testing.T) {
	reader, closer := setupMetricsReader(t)
	defer closer()

	qParams := &metricstore.LatenciesQueryParameters{}
	r, err := reader.GetLatencies(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetCallRates(t *testing.T) {
	reader, closer := setupMetricsReader(t)
	defer closer()

	qParams := &metricstore.CallRateQueryParameters{}
	r, err := reader.GetCallRates(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetErrorRates(t *testing.T) {
	reader, closer := setupMetricsReader(t)
	defer closer()

	qParams := &metricstore.ErrorRateQueryParameters{}
	r, err := reader.GetErrorRates(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetMinStepDuration(t *testing.T) {
	reader, closer := setupMetricsReader(t)
	defer closer()

	params := metricstore.MinStepDurationQueryParameters{}
	minStep, err := reader.GetMinStepDuration(context.Background(), &params)
	require.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}
