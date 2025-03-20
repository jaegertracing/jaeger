// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package benchmark_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	promExporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	prom "github.com/jaegertracing/jaeger/internal/metrics/prometheus"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func setupPrometheusFactory() api.Factory {
	reg := prometheus.NewRegistry()
	return prom.New(prom.WithRegisterer(reg))
}

func setupOTELFactory(b *testing.B) api.Factory {
	registry := prometheus.NewRegistry()
	exporter, err := promExporter.New(promExporter.WithRegisterer(registry))
	require.NoError(b, err)
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(exporter),
	)
	return otelmetrics.NewFactory(meterProvider)
}

func benchmarkCounter(b *testing.B, factory api.Factory) {
	counter := factory.Counter(api.Options{
		Name: "test_counter",
		Tags: map[string]string{"tag1": "value1"},
	})

	for i := 0; i < b.N; i++ {
		counter.Inc(1)
	}
}

func benchmarkGauge(b *testing.B, factory api.Factory) {
	gauge := factory.Gauge(api.Options{
		Name: "test_gauge",
		Tags: map[string]string{"tag1": "value1"},
	})

	for i := 0; i < b.N; i++ {
		gauge.Update(1)
	}
}

func benchmarkTimer(b *testing.B, factory api.Factory) {
	timer := factory.Timer(api.TimerOptions{
		Name: "test_timer",
		Tags: map[string]string{"tag1": "value1"},
	})

	for i := 0; i < b.N; i++ {
		timer.Record(100)
	}
}

func benchmarkHistogram(b *testing.B, factory api.Factory) {
	histogram := factory.Histogram(api.HistogramOptions{
		Name: "test_histogram",
		Tags: map[string]string{"tag1": "value1"},
	})

	for i := 0; i < b.N; i++ {
		histogram.Record(1.0)
	}
}

func BenchmarkPrometheusCounter(b *testing.B) {
	benchmarkCounter(b, setupPrometheusFactory())
}

func BenchmarkOTELCounter(b *testing.B) {
	benchmarkCounter(b, setupOTELFactory(b))
}

func BenchmarkPrometheusGauge(b *testing.B) {
	benchmarkGauge(b, setupPrometheusFactory())
}

func BenchmarkOTELGauge(b *testing.B) {
	benchmarkGauge(b, setupOTELFactory(b))
}

func BenchmarkPrometheusTimer(b *testing.B) {
	benchmarkTimer(b, setupPrometheusFactory())
}

func BenchmarkOTELTimer(b *testing.B) {
	benchmarkTimer(b, setupOTELFactory(b))
}

func BenchmarkPrometheusHistogram(b *testing.B) {
	benchmarkHistogram(b, setupPrometheusFactory())
}

func BenchmarkOTELHistogram(b *testing.B) {
	benchmarkHistogram(b, setupOTELFactory(b))
}
