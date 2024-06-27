// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics_test

import (
	"testing"
	"time"

	promReg "github.com/prometheus/client_golang/prometheus"
	promModel "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func newTestFactory(t *testing.T, registry *promReg.Registry) metrics.Factory {
	exporter, err := prometheus.New(prometheus.WithRegisterer(registry))
	require.NoError(t, err)
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	return otelmetrics.NewFactory(meterProvider)
}

func findMetric(t *testing.T, registry *promReg.Registry, name string) *promModel.MetricFamily {
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	for _, mf := range metricFamilies {
		t.Log(mf.GetName())
		if mf.GetName() == name {
			return mf
		}
	}
	require.NotNil(t, name, "Expected to find Metric Family")
	return nil
}

func TestCounter(t *testing.T) {
	registry := promReg.NewPedanticRegistry()
	factory := newTestFactory(t, registry)
	counter := factory.Counter(metrics.Options{
		Name: "test_counter",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, counter)
	counter.Inc(1)
	counter.Inc(1)

	testCounter := findMetric(t, registry, "test_counter_total")
	metrics := testCounter.GetMetric()
	assert.Equal(t, float64(2), metrics[0].GetCounter().GetValue())
}

func TestGauge(t *testing.T) {
	registry := promReg.NewPedanticRegistry()
	factory := newTestFactory(t, registry)
	gauge := factory.Gauge(metrics.Options{
		Name: "test_gauge",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, gauge)
	gauge.Update(2)

	testGauge := findMetric(t, registry, "test_gauge")

	metrics := testGauge.GetMetric()
	assert.Equal(t, float64(2), metrics[0].GetGauge().GetValue())
}

func TestHistogram(t *testing.T) {
	registry := promReg.NewPedanticRegistry()
	factory := newTestFactory(t, registry)
	histogram := factory.Histogram(metrics.HistogramOptions{
		Name: "test_histogram",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, histogram)
	histogram.Record(1.0)

	testHistogram := findMetric(t, registry, "test_histogram")

	metrics := testHistogram.GetMetric()
	assert.Equal(t, float64(1), metrics[0].GetHistogram().GetSampleSum())
}

func TestTimer(t *testing.T) {
	registry := promReg.NewPedanticRegistry()
	factory := newTestFactory(t, registry)
	timer := factory.Timer(metrics.TimerOptions{
		Name: "test_timer",
		Tags: map[string]string{"tag1": "value1"},
	})
	require.NotNil(t, timer)
	timer.Record(100 * time.Millisecond)

	testTimer := findMetric(t, registry, "test_timer_seconds")

	metrics := testTimer.GetMetric()
	assert.Equal(t, float64(0.1), metrics[0].GetHistogram().GetSampleSum())
}

func TestNamespace(t *testing.T) {
	registry := promReg.NewPedanticRegistry()
	factory := newTestFactory(t, registry)
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

	testCounter := findMetric(t, registry, "namespace_test_counter_total")

	metrics := testCounter.GetMetric()
	assert.Equal(t, float64(1), metrics[0].GetCounter().GetValue())
}

func TestNormalization(t *testing.T) {
	registry := promReg.NewPedanticRegistry()
	factory := newTestFactory(t, registry)
	normalizedFactory := factory.Namespace(metrics.NSOptions{
		Name: "My Namespace",
	})

	gauge := normalizedFactory.Gauge(metrics.Options{
		Name: "My Gauge",
	})
	require.NotNil(t, gauge)
	gauge.Update(1)

	testGauge := findMetric(t, registry, "My_Namespace_My_Gauge")

	metrics := testGauge.GetMetric()
	assert.Equal(t, float64(1), metrics[0].GetGauge().GetValue())
}
