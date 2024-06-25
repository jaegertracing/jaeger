// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package benchmark_test

import (
	"log"
	"testing"

	prometheus "github.com/prometheus/client_golang/prometheus"
	promExporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	prom "github.com/jaegertracing/jaeger/internal/metrics/prometheus"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func setupPrometheusFactory() metrics.Factory {
	reg := prometheus.NewRegistry()
	return prom.New(prom.WithRegisterer(reg))
}

func setupOTELFactory(b *testing.B) metrics.Factory {
	registry := prometheus.NewRegistry()
	exporter, err := promExporter.New(promExporter.WithRegisterer(registry))
	require.NoError(b, err)
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(exporter),
	)
	return otelmetrics.NewFactory(meterProvider)
}

func benchmarkMetric(b *testing.B, factory metrics.Factory, metricType string) {
	var metricObj interface{}
	switch metricType {
	case "counter":
		metricObj = factory.Counter(metrics.Options{
			Name: "test_counter",
			Tags: map[string]string{"tag1": "value1"},
		})
	case "gauge":
		metricObj = factory.Gauge(metrics.Options{
			Name: "test_gauge",
			Tags: map[string]string{"tag1": "value1"},
		})
	case "timer":
		metricObj = factory.Timer(metrics.TimerOptions{
			Name: "test_timer",
			Tags: map[string]string{"tag1": "value1"},
		})
	case "histogram":
		metricObj = factory.Histogram(metrics.HistogramOptions{
			Name: "test_histogram",
			Tags: map[string]string{"tag1": "value1"},
		})
	}

	for i := 0; i < b.N; i++ {
		switch m := metricObj.(type) {
		case metrics.Counter:
			m.Inc(1)
		case metrics.Gauge:
			m.Update(1)
		case metrics.Timer:
			m.Record(100)
		case metrics.Histogram:
			m.Record(1.0)
		}
	}
}

func BenchmarkPrometheusCounter(b *testing.B) {
	benchmarkMetric(b, setupPrometheusFactory(), "counter")
}

func BenchmarkOTELCounter(b *testing.B) {
	benchmarkMetric(b, setupOTELFactory(), "counter")
}

func BenchmarkPrometheusGauge(b *testing.B) {
	benchmarkMetric(b, setupPrometheusFactory(), "gauge")
}

func BenchmarkOTELGauge(b *testing.B) {
	benchmarkMetric(b, setupOTELFactory(), "gauge")
}

func BenchmarkPrometheusTimer(b *testing.B) {
	benchmarkMetric(b, setupPrometheusFactory(), "timer")
}

func BenchmarkOTELTimer(b *testing.B) {
	benchmarkMetric(b, setupOTELFactory(), "timer")
}

func BenchmarkPrometheusHistogram(b *testing.B) {
	benchmarkMetric(b, setupPrometheusFactory(), "histogram")
}

func BenchmarkOTELHistogram(b *testing.B) {
	benchmarkMetric(b, setupOTELFactory(), "histogram")
}
