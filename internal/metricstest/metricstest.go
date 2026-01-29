// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metricstest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ExpectedMetric contains metrics under test.
type ExpectedMetric struct {
	Name  string
	Tags  map[string]string
	Value int
}

// ExpectedTimerMetric contains timer metrics under test.
type ExpectedTimerMetric struct {
	Name       string
	Tags       map[string]string
	Percentile string // e.g., "P50", "P75", "P90", "P95", "P99", "P999"
	Value      int    // expected value in milliseconds
}

// AssertTimerMetrics checks if timer metrics exist with expected percentile values.
func (f *Factory) AssertTimerMetrics(t *testing.T, expectedMetrics ...ExpectedTimerMetric) {
	_, gauges := f.Snapshot()
	for _, expected := range expectedMetrics {
		key := GetKey(expected.Name, expected.Tags, "|", "=")
		fullKey := key + "." + expected.Percentile
		assert.EqualValues(t,
			expected.Value,
			gauges[fullKey],
			"expected timer metric name=%s percentile=%s tags: %+v; got: %+v",
			expected.Name, expected.Percentile, expected.Tags, gauges,
		)
	}
}

// AssertCounterMetrics checks if counter metrics exist.
func (f *Factory) AssertCounterMetrics(t *testing.T, expectedMetrics ...ExpectedMetric) {
	counters, _ := f.Snapshot()
	assertMetrics(t, counters, expectedMetrics...)
}

// AssertGaugeMetrics checks if gauge metrics exist.
func (f *Factory) AssertGaugeMetrics(t *testing.T, expectedMetrics ...ExpectedMetric) {
	_, gauges := f.Snapshot()
	assertMetrics(t, gauges, expectedMetrics...)
}

func assertMetrics(t *testing.T, actualMetrics map[string]int64, expectedMetrics ...ExpectedMetric) {
	for _, expected := range expectedMetrics {
		key := GetKey(expected.Name, expected.Tags, "|", "=")
		assert.EqualValues(t,
			expected.Value,
			actualMetrics[key],
			"expected metric name=%s tags: %+v; got: %+v", expected.Name, expected.Tags, actualMetrics,
		)
	}
}
