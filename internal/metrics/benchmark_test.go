// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package benchmark_test

import (
	"context"
	"testing"

	promsdk "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/bridge/opencensus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	promExporter "go.opentelemetry.io/otel/exporters/prometheus"
)

func BenchmarkPrometheusCounter(b *testing.B) {
	reg := promsdk.NewRegistry()
	opts := promsdk.CounterOpts{
		Name: "test_counter",
		Help: "help",
	}
	cv := promsdk.NewCounterVec(opts, []string{"tag1"})
	reg.MustRegister(cv)
	counter := cv.WithLabelValues("value1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(1)
	}
}

func BenchmarkOTELCounter(b *testing.B) {
	counter := otelCounter(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(ctx, 1)
	}
}

func BenchmarkCencusCounter(b *testing.B) {
	counter := censusCounter(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(ctx, 1)
	}
}

func BenchmarkOTELCounterWithLabel(b *testing.B) {
	counter := otelCounter(b)
	ctx := context.Background()
	attrSet := attribute.NewSet(attribute.String("tag1", "value1"))
	attrOpt := metric.WithAttributeSet(attrSet)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(ctx, 1, attrOpt)
	}
}
func BenchmarkCencusCounterWithLabel(b *testing.B) {
	counter := censusCounter(b)
	ctx := context.Background()
	attrSet := attribute.NewSet(attribute.String("tag1", "value1"))
	attrOpt := metric.WithAttributeSet(attrSet)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(ctx, 1, attrOpt)
	}
}

func otelCounter(b *testing.B) metric.Int64Counter {
	registry := promsdk.NewRegistry()
	exporter, err := promExporter.New(promExporter.WithRegisterer(registry))
	require.NoError(b, err)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)

	meter := meterProvider.Meter("test")
	counter, err := meter.Int64Counter("test_counter")
	require.NoError(b, err)
	return counter
}

func censusCounter(b *testing.B) metric.Int64Counter {
	registry := promsdk.NewRegistry()
	exporter, err := promExporter.New(
		promExporter.WithRegisterer(registry),
		promExporter.WithProducer(opencensus.NewMetricProducer()),
	)
	require.NoError(b, err)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)

	meter := meterProvider.Meter("test")
	counter, err := meter.Int64Counter("test_counter")
	require.NoError(b, err)
	return counter
}
