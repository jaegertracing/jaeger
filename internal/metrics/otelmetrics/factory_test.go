// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func newTestFactory(t *testing.T) metrics.Factory {
	exporter, err := prometheus.New()
	require.NoError(t, err)
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	return otelmetrics.NewFactory(meterProvider)
}

func TestCounter(t *testing.T) {
	factory := newTestFactory()
	counter := factory.Counter(metrics.Options{
		Name: "test_counter",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, counter)
	counter.Inc(1)
}

func TestGauge(t *testing.T) {
	factory := newTestFactory()
	gauge := factory.Gauge(metrics.Options{
		Name: "test_gauge",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, gauge)
	gauge.Update(1)
}

func TestHistogram(t *testing.T) {
	factory := newTestFactory()
	histogram := factory.Histogram(metrics.HistogramOptions{
		Name: "test_histogram",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, histogram)
	histogram.Record(1.0)
}

func TestTimer(t *testing.T) {
	factory := newTestFactory()
	timer := factory.Timer(metrics.TimerOptions{
		Name: "test_timer",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, timer)
	timer.Record(100 * time.Millisecond)
}

func TestNamespace(t *testing.T) {
	factory := newTestFactory()
	nsFactory := factory.Namespace(metrics.NSOptions{
		Name: "namespace",
		Tags: map[string]string{"ns_tag1": "ns_value1"},
	})

	counter := nsFactory.Counter(metrics.Options{
		Name: "test_counter",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, counter)
	counter.Inc(1)
}

func TestNormalization(t *testing.T) {
	factory := newTestFactory()
	normalizedFactory := factory.Namespace(metrics.NSOptions{
		Name: "My Namespace",
	})

	gauge := normalizedFactory.Gauge(metrics.Options{
		Name: "My Gauge",
	})
	require.NotNil(t, gauge)
}

func TestNoopMeterProvider(t *testing.T) {
	noOpFactory := otelmetrics.NewFactory(noop.NewMeterProvider())
	counter := noOpFactory.Counter(metrics.Options{
		Name: "noop_counter",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, counter)
	counter.Inc(1)
}
