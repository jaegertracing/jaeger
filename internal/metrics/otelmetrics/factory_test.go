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
	exporter, err := prometheus.New(prometheus.WithRegisterer(registry), prometheus.WithoutScopeInfo())
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
	require.Fail(t, "Expected to find Metric Family")
	return nil
}

func promLabelsToMap(labels []*promModel.LabelPair) map[string]string {
	labelMap := make(map[string]string)
	for _, label := range labels {
		labelMap[label.GetName()] = label.GetValue()
	}
	return labelMap
}

func TestInvalidCounter(t *testing.T) {
	factory := newTestFactory(t, promReg.NewPedanticRegistry())
	counter := factory.Counter(metrics.Options{
		Name: "invalid*counter%",
	})
	assert.Equal(t, counter, metrics.NullCounter, "Expected NullCounter, got %v", counter)
}

func TestInvalidGauge(t *testing.T) {
	factory := newTestFactory(t, promReg.NewPedanticRegistry())
	gauge := factory.Gauge(metrics.Options{
		Name: "#invalid>gauge%",
	})
	assert.Equal(t, gauge, metrics.NullGauge, "Expected NullCounter, got %v", gauge)
}

func TestInvalidHistogram(t *testing.T) {
	factory := newTestFactory(t, promReg.NewPedanticRegistry())
	histogram := factory.Histogram(metrics.HistogramOptions{
		Name: "invalid>histogram?%",
	})
	assert.Equal(t, histogram, metrics.NullHistogram, "Expected NullCounter, got %v", histogram)
}

func TestInvalidTimer(t *testing.T) {
	factory := newTestFactory(t, promReg.NewPedanticRegistry())
	timer := factory.Timer(metrics.TimerOptions{
		Name: "invalid*<=timer%",
	})
	assert.Equal(t, timer, metrics.NullTimer, "Expected NullCounter, got %v", timer)
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
	assert.InDelta(t, float64(2), metrics[0].GetCounter().GetValue(), 0.01)
	expectedLabels := map[string]string{
		"tag1": "value1",
	}
	assert.Equal(t, expectedLabels, promLabelsToMap(metrics[0].GetLabel()))
}

func TestCounterNamingConvention(t *testing.T) {
	input := "test_counter"
	expected := "test_counter_total"

	result := otelmetrics.CounterNamingConvention(input)

	if result != expected {
		t.Errorf("Expected %s, but got %s", expected, result)
	}
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
	assert.InDelta(t, float64(2), metrics[0].GetGauge().GetValue(), 0.01)
	expectedLabels := map[string]string{
		"tag1": "value1",
	}
	assert.Equal(t, expectedLabels, promLabelsToMap(metrics[0].GetLabel()))
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
	assert.InDelta(t, float64(1), metrics[0].GetHistogram().GetSampleSum(), 0.01)
	expectedLabels := map[string]string{
		"tag1": "value1",
	}
	assert.Equal(t, expectedLabels, promLabelsToMap(metrics[0].GetLabel()))
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
	assert.InDelta(t, float64(0.1), metrics[0].GetHistogram().GetSampleSum(), 0.01)
	expectedLabels := map[string]string{
		"tag1": "value1",
	}
	assert.Equal(t, expectedLabels, promLabelsToMap(metrics[0].GetLabel()))
}

func TestNamespace(t *testing.T) {
	testCases := []struct {
		name           string
		nsOptions1     metrics.NSOptions
		nsOptions2     metrics.NSOptions
		expectedName   string
		expectedLabels map[string]string
	}{
		{
			name: "Nested Namespace",
			nsOptions1: metrics.NSOptions{
				Name: "first_namespace",
				Tags: map[string]string{"ns_tag1": "ns_value1"},
			},
			nsOptions2: metrics.NSOptions{
				Name: "second_namespace",
				Tags: map[string]string{"ns_tag3": "ns_value3"},
			},
			expectedName: "first_namespace_second_namespace_test_counter_total",
			expectedLabels: map[string]string{
				"ns_tag1": "ns_value1",
				"ns_tag3": "ns_value3",
				"tag1":    "value1",
			},
		},
		{
			name: "Single Namespace",
			nsOptions1: metrics.NSOptions{
				Name: "single_namespace",
				Tags: map[string]string{"ns_tag2": "ns_value2"},
			},
			nsOptions2:   metrics.NSOptions{},
			expectedName: "single_namespace_test_counter_total",
			expectedLabels: map[string]string{
				"ns_tag2": "ns_value2",
				"tag1":    "value1",
			},
		},
		{
			name:         "Empty Namespace Name",
			nsOptions1:   metrics.NSOptions{},
			nsOptions2:   metrics.NSOptions{},
			expectedName: "test_counter_total",
			expectedLabels: map[string]string{
				"tag1": "value1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry := promReg.NewPedanticRegistry()
			factory := newTestFactory(t, registry)
			nsFactory1 := factory.Namespace(tc.nsOptions1)
			nsFactory2 := nsFactory1.Namespace(tc.nsOptions2)

			counter := nsFactory2.Counter(metrics.Options{
				Name: "test_counter",
				Tags: map[string]string{"tag1": "value1"},
			})
			require.NotNil(t, counter)
			counter.Inc(1)

			testCounter := findMetric(t, registry, tc.expectedName)

			metrics := testCounter.GetMetric()
			assert.InDelta(t, float64(1), metrics[0].GetCounter().GetValue(), 0.01)
			assert.Equal(t, tc.expectedLabels, promLabelsToMap(metrics[0].GetLabel()))
		})
	}
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
	assert.InDelta(t, float64(1), metrics[0].GetGauge().GetValue(), 0.01)
}
