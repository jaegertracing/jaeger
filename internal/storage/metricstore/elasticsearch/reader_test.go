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
func tracerProvider(t *testing.T) trace.TracerProvider {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	})
	return tp
}

func clientProvider(t *testing.T, c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) es.Client {
	client, err := config.NewClient(c, logger, metricsFactory)
	require.NoError(t, err)
	require.NotNil(t, client)

	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}

// setupMetricsReader provides a common setup for tests requiring a MetricsReader.
func setupMetricsReader(t *testing.T) *MetricsReader {
	logger := zap.NewNop()
	tracer := tracerProvider(t)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockServerResponse)
	}))
	t.Cleanup(mockServer.Close)
	cfg := config.Configuration{
		Servers:  []string{mockServer.URL},
		LogLevel: "debug",
	}

	client := clientProvider(t, &cfg, logger, telemetry.NoopSettings().Metrics)
	reader := NewMetricsReader(client, logger, tracer)

	require.NotNil(t, reader)
	return reader
}

func TestGetLatencies(t *testing.T) {
	reader := setupMetricsReader(t)

	qParams := &metricstore.LatenciesQueryParameters{}
	r, err := reader.GetLatencies(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetCallRates(t *testing.T) {
	reader := setupMetricsReader(t)

	qParams := &metricstore.CallRateQueryParameters{}
	r, err := reader.GetCallRates(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetErrorRates(t *testing.T) {
	reader := setupMetricsReader(t)

	qParams := &metricstore.ErrorRateQueryParameters{}
	r, err := reader.GetErrorRates(context.Background(), qParams)
	assert.Zero(t, r)
	require.ErrorIs(t, err, ErrNotImplemented)
	require.EqualError(t, err, ErrNotImplemented.Error())
}

func TestGetMinStepDuration(t *testing.T) {
	reader := setupMetricsReader(t)

	params := metricstore.MinStepDurationQueryParameters{}
	minStep, err := reader.GetMinStepDuration(context.Background(), &params)
	require.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}
