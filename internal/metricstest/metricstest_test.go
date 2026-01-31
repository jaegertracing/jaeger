// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstest

import (
	"testing"
	"time"
)

func TestAssertMetrics(t *testing.T) {
	f := NewFactory(0)
	tags := map[string]string{"key": "value"}
	f.IncCounter("counter", tags, 1)
	f.UpdateGauge("gauge", tags, 11)

	f.AssertCounterMetrics(t, ExpectedMetric{Name: "counter", Tags: tags, Value: 1})
	f.AssertGaugeMetrics(t, ExpectedMetric{Name: "gauge", Tags: tags, Value: 11})
}

func TestAssertTimerMetrics(t *testing.T) {
	f := NewFactory(0)
	tags := map[string]string{"service": "test"}

	// Record some timer values (in milliseconds: 10, 20, 30, 40, 50)
	f.RecordTimer("request_duration", tags, 10*time.Millisecond)
	f.RecordTimer("request_duration", tags, 20*time.Millisecond)
	f.RecordTimer("request_duration", tags, 30*time.Millisecond)
	f.RecordTimer("request_duration", tags, 40*time.Millisecond)
	f.RecordTimer("request_duration", tags, 50*time.Millisecond)

	// With 5 values [10, 20, 30, 40, 50]:
	// P50 = sorted[int(4 * 0.50)] = sorted[2] = 30
	// P99 = sorted[int(4 * 0.99)] = sorted[3] = 40
	f.AssertTimerMetrics(t,
		ExpectedTimerMetric{Name: "request_duration", Tags: tags, Percentile: "P50", Value: 30},
		ExpectedTimerMetric{Name: "request_duration", Tags: tags, Percentile: "P99", Value: 40},
	)
}
