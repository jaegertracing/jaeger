// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"math"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

// TestProcessLatencies tests the ProcessLatencies function for scaling and handling edge cases.
func TestProcessLatencies(t *testing.T) {
	tests := []struct {
		name     string
		input    *metrics.MetricFamily
		expected float64
		isNaN    bool
	}{
		{
			name: "should scale microseconds to milliseconds",
			input: createMetricFamily("service_latencies", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(time.Now(), 1500.0),
				}),
			}),
			expected: 1.5,
			isNaN:    false,
		},
		{
			name: "should handle NaN values",
			input: createMetricFamily("service_latencies", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(time.Now(), math.NaN()),
				}),
			}),
			isNaN: true,
		},
		{
			name:  "should handle empty metrics",
			input: createMetricFamily("service_latencies", []*metrics.Metric{}),
			isNaN: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScaleAndRoundLatencies(tt.input)
			if len(result.Metrics) == 0 || len(result.Metrics[0].MetricPoints) == 0 {
				assert.True(t, tt.isNaN)
				return
			}
			value := result.Metrics[0].MetricPoints[0].GetGaugeValue().GetDoubleValue()
			if tt.isNaN {
				assert.True(t, math.IsNaN(value))
			} else {
				assert.InDelta(t, tt.expected, value, 0.1)
			}
		})
	}
}

// TestProcessCallRates tests the ProcessCallRates function for rate calculation and trimming.
func TestProcessCallRates(t *testing.T) {
	now := time.Now()
	params := metricstore.BaseQueryParameters{
		Step:    ptr(time.Second * 10),
		RatePer: ptr(time.Minute),
	}
	timeRange := TimeRange{startTimeMillis: now.Add(-time.Minute).UnixMilli()}

	tests := []struct {
		name           string
		input          *metrics.MetricFamily
		expectedPoints int
		expectedValue  float64
		isNaN          bool
	}{
		{
			name: "should calculate call rates and trim points",
			input: createMetricFamily("service_call_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now.Add(-2*time.Minute), 10.0),
					createMetricPoint(now.Add(-time.Minute), 20.0),
					createMetricPoint(now, 30.0),
				}),
			}),
			expectedPoints: 2,
			isNaN:          true,
		},
		{
			name: "should handle insufficient window",
			input: createMetricFamily("service_call_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 10.0),
				}),
			}),
			expectedPoints: 1,
			isNaN:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCallRates(tt.input, params, timeRange)
			assert.Len(t, result.Metrics[0].MetricPoints, tt.expectedPoints)
			assert.True(t, math.IsNaN(result.Metrics[0].MetricPoints[0].GetGaugeValue().GetDoubleValue()))
		})
	}
}

// TestProcessErrorRates tests the ProcessErrorRates function for error rate calculation.
func TestProcessErrorRates(t *testing.T) {
	now := time.Now()
	params := metricstore.BaseQueryParameters{
		Step:    ptr(time.Minute),
		RatePer: ptr(time.Minute),
	}
	timeRange := TimeRange{startTimeMillis: now.Add(-time.Minute).UnixMilli()}

	tests := []struct {
		name         string
		errorMetrics *metrics.MetricFamily
		callMetrics  *metrics.MetricFamily
		expected     float64
		isNaN        bool
	}{
		{
			name: "should calculate error rates correctly",
			errorMetrics: createMetricFamily("service_error_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 5.0),
				}),
			}),
			callMetrics: createMetricFamily("service_call_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 10.0),
				}),
			}),
			expected: 0.0, // No rate during this time
			isNaN:    false,
		},
		{
			name: "should handle division by zero",
			errorMetrics: createMetricFamily("service_error_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 5.0),
				}),
			}),
			callMetrics: createMetricFamily("service_call_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 0.0),
				}),
			}),
			expected: 0.0,
			isNaN:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateErrorRates(tt.errorMetrics, tt.callMetrics, params, timeRange)
			value := result.Metrics[0].MetricPoints[0].GetGaugeValue().GetDoubleValue()
			if tt.isNaN {
				assert.True(t, math.IsNaN(value))
			} else {
				assert.InDelta(t, tt.expected, value, 0.1)
			}
		})
	}
}

// TestCalculateErrorRateValue tests the calculateErrorRateValue function for edge cases.
func TestCalculateErrorRateValue(t *testing.T) {
	tests := []struct {
		name     string
		errorVal float64
		callVal  float64
		expected float64
		isNaN    bool
	}{
		{
			name:     "normal case",
			errorVal: 5.0,
			callVal:  10.0,
			expected: 0.5,
			isNaN:    false,
		},
		{
			name:     "error NaN, call valid",
			errorVal: math.NaN(),
			callVal:  10.0,
			expected: 0.0,
			isNaN:    false,
		},
		{
			name:     "error valid, call NaN",
			errorVal: 5.0,
			callVal:  math.NaN(),
			isNaN:    true,
		},
		{
			name:     "both NaN",
			errorVal: math.NaN(),
			callVal:  math.NaN(),
			isNaN:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorPoint := createMetricPoint(time.Now(), tt.errorVal)
			callPoint := createMetricPoint(time.Now(), tt.callVal)
			result := calculateErrorRateValue(errorPoint, callPoint)
			if tt.isNaN {
				assert.True(t, math.IsNaN(result))
			} else {
				assert.InDelta(t, tt.expected, result, 0.1)
			}
		})
	}
}

// TestTrimMetricPointsBefore tests the trimMetricPointsBefore function for trimming points.
func TestTrimMetricPointsBefore(t *testing.T) {
	now := time.Now()
	input := createMetricFamily("test_metrics", []*metrics.Metric{
		createMetric([]*metrics.MetricPoint{
			createMetricPoint(now.Add(-2*time.Minute), 10.0),
			createMetricPoint(now.Add(-time.Minute), 20.0),
			createMetricPoint(now, 30.0),
		}),
	})

	result := trimMetricPointsBefore(input, now.Add(-90*time.Second).UnixMilli())
	assert.Len(t, result.Metrics[0].MetricPoints, 2)
}

func TestZeroValue(t *testing.T) {
	// Create test input with one NaN and one non-NaN value
	input := []*metrics.MetricPoint{
		{Value: toDomainMetricPointValue(math.NaN())}, // NaN case
		{Value: toDomainMetricPointValue(42.0)},       // Non-NaN case
	}

	result := zeroValue(input)

	assert.True(t, math.IsNaN(result[0].GetGaugeValue().GetDoubleValue()))
	assert.InDelta(t, 0.0, result[1].GetGaugeValue().GetDoubleValue(), 0.1)
}

// createMetricFamily creates a MetricFamily with the given name and metrics.
func createMetricFamily(name string, m []*metrics.Metric) *metrics.MetricFamily {
	return &metrics.MetricFamily{
		Name:    name,
		Type:    metrics.MetricType_GAUGE,
		Help:    name + " metrics",
		Metrics: m,
	}
}

// createMetric creates a Metric with the given metric points.
func createMetric(points []*metrics.MetricPoint) *metrics.Metric {
	return &metrics.Metric{
		MetricPoints: points,
	}
}

// createMetricPoint creates a MetricPoint with the given timestamp and value.
func createMetricPoint(ts time.Time, value float64) *metrics.MetricPoint {
	timestamp, _ := types.TimestampProto(ts)
	return &metrics.MetricPoint{
		Timestamp: timestamp,
		Value:     toDomainMetricPointValue(value),
	}
}

// ptr returns a pointer to the given time.Duration.
func ptr(d time.Duration) *time.Duration {
	return &d
}
