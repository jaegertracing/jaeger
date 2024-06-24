// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package benchmark_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	prom "github.com/jaegertracing/jaeger/internal/metrics/prometheus"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func benchmarkCounter(b *testing.B, factory metrics.Factory) {
	counter := factory.Counter(metrics.Options{
		Name: "test_counter",
		Tags: map[string]string{"tag1": "value1"},
	})

	for i := 0; i < b.N; i++ {
		counter.Inc(1)
	}
}

func BenchmarkPrometheusCounter(b *testing.B) {
	reg := prometheus.NewRegistry()
	factory := prom.New(prom.WithRegisterer(reg))
	benchmarkCounter(b, factory)
}

func BenchmarkOTELCounter(b *testing.B) {
	meterProvider := metric.NewMeterProvider()
	factory := otelmetrics.NewFactory(meterProvider)
	benchmarkCounter(b, factory)
}
