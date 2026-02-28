// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleNameNormalizer(t *testing.T) {
	n := &SimpleNameNormalizer{
		SafeSets: []SafeCharacterSet{
			&Range{From: 'a', To: 'z'},
			&Char{'-'},
		},
		Replacement: '-',
	}
	assert.Equal(t, "ab-cd", n.Normalize("ab-cd"), "all valid")
	assert.Equal(t, "ab-cd", n.Normalize("ab.cd"), "single mismatch")
	assert.Equal(t, "a--cd", n.Normalize("aB-cd"), "range letter mismatch")
}


// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"testing"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestSimpleNameNormalizer(t *testing.T) {
	n := &SimpleNameNormalizer{
		SafeSets: []SafeCharacterSet{
			&Range{From: 'a', To: 'z'},
			&Char{'-'},
		},
		Replacement: '-',
	}
	assert.Equal(t, "ab-cd", n.Normalize("ab-cd"), "all valid")
	assert.Equal(t, "ab-cd", n.Normalize("ab.cd"), "single mismatch")
	assert.Equal(t, "a--cd", n.Normalize("aB-cd"), "range letter mismatch")
}

// TestNormalizeMetricFamiliesForE2E verifies that the namespace label is correctly
// normalized to a stable value for end-to-end testing scenarios.
func TestNormalizeMetricFamiliesForE2E(t *testing.T) {
	// Arrange: Create a mock MetricFamily with a randomized namespace label.
	randomNamespace := "hotrod-e2e-random-12345"
	mfWithNamespace := &io_prometheus_client.MetricFamily{
		Name: stringPtr("my_metric_total"),
		Help: stringPtr("A test metric."),
		Type: io_prometheus_client.MetricType_COUNTER.Enum(),
		Metric: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{
					{Name: stringPtr("service"), Value: stringPtr("driver")},
					{Name: stringPtr("namespace"), Value: stringPtr(randomNamespace)}, // This should be normalized
					{Name: stringPtr("status"), Value: stringPtr("ok")},
				},
				Counter: &io_prometheus_client.Counter{
					Value: float64Ptr(123.0),
				},
			},
			{
				Label: []*io_prometheus_client.LabelPair{
					{Name: stringPtr("service"), Value: stringPtr("customer")},
					{Name: stringPtr("namespace"), Value: stringPtr("another-random-ns")}, // This should also be normalized
				},
				Counter: &io_prometheus_client.Counter{
					Value: float64Ptr(45.0),
				},
			},
		},
	}

	// Arrange: Create another mock MetricFamily without a namespace label to ensure it's not affected.
	mfWithoutNamespace := &io_prometheus_client.MetricFamily{
		Name: stringPtr("other_metric_gauge"),
		Help: stringPtr("Another test metric."),
		Type: io_prometheus_client.MetricType_GAUGE.Enum(),
		Metric: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{
					{Name: stringPtr("method"), Value: stringPtr("get")},
				},
				Gauge: &io_prometheus_client.Gauge{
					Value: float64Ptr(99.0),
				},
			},
		},
	}

	metricFamilies := []*io_prometheus_client.MetricFamily{mfWithNamespace, mfWithoutNamespace}

	// Act: Call the normalization function.
	normalizedFamilies := NormalizeMetricFamiliesForE2E(metricFamilies)

	// Assert
	assert.NotNil(t, normalizedFamilies, "Normalized families should not be nil")
	assert.Len(t, normalizedFamilies, 2, "Should have two metric families after normalization")

	// Assert the first metric family (mfWithNamespace) was normalized.
	normalizedMfWithNamespace := normalizedFamilies[0]
	assert.Equal(t, "my_metric_total", *normalizedMfWithNamespace.Name, "Metric name should be unchanged")
	assert.Len(t, normalizedMfWithNamespace.Metric, 2, "Should have two metrics in the first family")

	// Check the first metric within mfWithNamespace
	metric1 := normalizedMfWithNamespace.Metric[0]
	assert.Equal(t, 123.0, *metric1.Counter.Value, "Counter value should be unchanged")
	expectedLabels1 := map[string]string{
		"service":   "driver",
		"namespace": E2EStableNamespace,
		"status":    "ok",
	}
	assert.Equal(t, expectedLabels1, labelsToMap(metric1.Label), "Labels for metric 1 should be correct")

	// Check the second metric within mfWithNamespace
	metric2 := normalizedMfWithNamespace.Metric[1]
	assert.Equal(t, 45.0, *metric2.Counter.Value, "Counter value should be unchanged")
	expectedLabels2 := map[string]string{
		"service":   "customer",
		"namespace": E2EStableNamespace,
	}
	assert.Equal(t, expectedLabels2, labelsToMap(metric2.Label), "Labels for metric 2 should be correct")

	// Assert the second metric family (mfWithoutNamespace) was not affected.
	normalizedMfWithoutNamespace := normalizedFamilies[1]
	assert.Equal(t, "other_metric_gauge", *normalizedMfWithoutNamespace.Name, "Metric name should be unchanged")
	assert.Len(t, normalizedMfWithoutNamespace.Metric, 1, "Should have one metric in the second family")
	metric3 := normalizedMfWithoutNamespace.Metric[0]
	assert.Equal(t, 99.0, *metric3.Gauge.Value, "Gauge value should be unchanged")
	expectedLabels3 := map[string]string{
		"method": "get",
	}
	assert.Equal(t, expectedLabels3, labelsToMap(metric3.Label), "Labels for metric 3 should be correct")
}

// labelsToMap is a test helper to convert a slice of LabelPair protos to a map.
func labelsToMap(labels []*io_prometheus_client.LabelPair) map[string]string {
	m := make(map[string]string, len(labels))
	for _, lp := range labels {
		m[*lp.Name] = *lp.Value
	}
	return m
}

// Helper functions for creating pointers to primitive types for protobufs
func stringPtr(s string) *string   { return &s }
func float64Ptr(f float64) *float64 { return &f }