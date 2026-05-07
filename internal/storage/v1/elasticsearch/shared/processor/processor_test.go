// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"math"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

// TestScaleAndRoundLatencies tests the ScaleAndRoundLatencies function for scaling and handling edge cases.
func TestScaleAndRoundLatencies(t *testing.T) {
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

// TestCalculateCallRates tests the CalculateCallRates function for rate calculation and trimming.
func TestCalculateCallRates(t *testing.T) {
	now := time.Now()
	step := 10 * time.Second
	ratePer := time.Minute
	params := metricstore.BaseQueryParameters{
		Step:    &step,
		RatePer: &ratePer,
	}
	timeRange := TimeRange{StartTimeMillis: now.Add(-time.Minute).UnixMilli()}

	tests := []struct {
		name           string
		input          *metrics.MetricFamily
		expectedPoints int
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
		},
		{
			name: "should handle insufficient window",
			input: createMetricFamily("service_call_rate", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 10.0),
				}),
			}),
			expectedPoints: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCallRates(tt.input, params, timeRange)
			assert.Len(t, result.Metrics[0].MetricPoints, tt.expectedPoints)
			// All points should be NaN due to insufficient window for rate calculation
			assert.True(t, math.IsNaN(result.Metrics[0].MetricPoints[0].GetGaugeValue().GetDoubleValue()))
		})
	}
}

// TestCalculateErrorRates tests the CalculateErrorRates function for error rate calculation.
func TestCalculateErrorRates(t *testing.T) {
	now := time.Now()
	params := metricstore.BaseQueryParameters{
		Step:    new(time.Minute),
		RatePer: new(time.Minute),
	}
	timeRange := TimeRange{StartTimeMillis: now.Add(-time.Minute).UnixMilli()}

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

// TestCalculateErrorRateValue tests the CalculateErrorRateValue function for edge cases.
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
			result := CalculateErrorRateValue(errorPoint, callPoint)
			if tt.isNaN {
				assert.True(t, math.IsNaN(result))
			} else {
				assert.InDelta(t, tt.expected, result, 0.1)
			}
		})
	}
}

// TestTrimMetricPointsBefore tests the TrimMetricPointsBefore function for trimming points.
func TestTrimMetricPointsBefore(t *testing.T) {
	now := time.Now()
	input := createMetricFamily("test_metrics", []*metrics.Metric{
		createMetric([]*metrics.MetricPoint{
			createMetricPoint(now.Add(-2*time.Minute), 10.0),
			createMetricPoint(now.Add(-time.Minute), 20.0),
			createMetricPoint(now, 30.0),
		}),
	})

	result := TrimMetricPointsBefore(input, now.Add(-90*time.Second).UnixMilli())
	assert.Len(t, result.Metrics[0].MetricPoints, 2)
}

func TestZeroValue(t *testing.T) {
	// Create test input with one NaN and one non-NaN value
	input := []*metrics.MetricPoint{
		{Value: toDomainMetricPointValue(math.NaN())}, // NaN case
		{Value: toDomainMetricPointValue(42.0)},       // Non-NaN case
	}

	result := ZeroValue(input)

	assert.True(t, math.IsNaN(result[0].GetGaugeValue().GetDoubleValue()))
	assert.InDelta(t, 0.0, result[1].GetGaugeValue().GetDoubleValue(), 0.1)
}

func TestScaleToMillisAndRound_EmptyWindow(t *testing.T) {
	var window []*metrics.MetricPoint
	result := ScaleToMillisAndRound(nil, window)
	assert.True(t, math.IsNaN(result))
}

// TestCalcErrorRates tests the CalcErrorRates function comprehensively.
func TestCalcErrorRates(t *testing.T) {
	now := time.Now()
	timestamp, _ := types.TimestampProto(now)

	tests := []struct {
		name         string
		errorMetrics *metrics.MetricFamily
		callMetrics  *metrics.MetricFamily
		expected     map[string]float64 // labelKey -> expected value
	}{
		{
			name: "matching labels with valid values",
			errorMetrics: &metrics.MetricFamily{
				Name: "errors",
				Type: metrics.MetricType_GAUGE,
				Metrics: []*metrics.Metric{
					{
						Labels: []*metrics.Label{{Name: "service", Value: "svc1"}},
						MetricPoints: []*metrics.MetricPoint{
							{Timestamp: timestamp, Value: toDomainMetricPointValue(5.0)},
						},
					},
				},
			},
			callMetrics: &metrics.MetricFamily{
				Name: "calls",
				Type: metrics.MetricType_GAUGE,
				Metrics: []*metrics.Metric{
					{
						Labels: []*metrics.Label{{Name: "service", Value: "svc1"}},
						MetricPoints: []*metrics.MetricPoint{
							{Timestamp: timestamp, Value: toDomainMetricPointValue(10.0)},
						},
					},
				},
			},
			expected: map[string]float64{
				"service=svc1": 0.5,
			},
		},
		{
			name: "call metric without matching error metric",
			errorMetrics: &metrics.MetricFamily{
				Name:    "errors",
				Type:    metrics.MetricType_GAUGE,
				Metrics: []*metrics.Metric{},
			},
			callMetrics: &metrics.MetricFamily{
				Name: "calls",
				Type: metrics.MetricType_GAUGE,
				Metrics: []*metrics.Metric{
					{
						Labels: []*metrics.Label{{Name: "service", Value: "svc2"}},
						MetricPoints: []*metrics.MetricPoint{
							{Timestamp: timestamp, Value: toDomainMetricPointValue(10.0)},
						},
					},
				},
			},
			expected: map[string]float64{
				"service=svc2": 0.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalcErrorRates(tt.errorMetrics, tt.callMetrics)
			assert.NotNil(t, result)
			assert.Len(t, result.Metrics, len(tt.expected))
			for _, metric := range result.Metrics {
				labelKey := GetLabelKey(metric.Labels)
				expectedVal, ok := tt.expected[labelKey]
				assert.True(t, ok, "unexpected label key: %s", labelKey)
				if len(metric.MetricPoints) > 0 {
					actualVal := metric.MetricPoints[0].GetGaugeValue().GetDoubleValue()
					assert.InDelta(t, expectedVal, actualVal, 0.01)
				}
			}
		})
	}
}

// TestCalculateErrorRatePoints tests the CalculateErrorRatePoints function.
func TestCalculateErrorRatePoints(t *testing.T) {
	now := time.Now()
	timestamp1, _ := types.TimestampProto(now)
	timestamp2, _ := types.TimestampProto(now.Add(time.Minute))

	tests := []struct {
		name        string
		errorPoints []*metrics.MetricPoint
		callPoints  []*metrics.MetricPoint
		expected    int // number of result points
	}{
		{
			name: "matching timestamps",
			errorPoints: []*metrics.MetricPoint{
				{Timestamp: timestamp1, Value: toDomainMetricPointValue(5.0)},
			},
			callPoints: []*metrics.MetricPoint{
				{Timestamp: timestamp1, Value: toDomainMetricPointValue(10.0)},
			},
			expected: 1,
		},
		{
			name: "non-matching timestamps",
			errorPoints: []*metrics.MetricPoint{
				{Timestamp: timestamp1, Value: toDomainMetricPointValue(5.0)},
			},
			callPoints: []*metrics.MetricPoint{
				{Timestamp: timestamp2, Value: toDomainMetricPointValue(10.0)},
			},
			expected: 0, // no matching timestamps, so no points
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateErrorRatePoints(tt.errorPoints, tt.callPoints)
			assert.Len(t, result, tt.expected)
		})
	}
}

// TestGetLabelKey tests the GetLabelKey function with various label combinations.
func TestGetLabelKey(t *testing.T) {
	tests := []struct {
		name     string
		labels   []*metrics.Label
		expected string
	}{
		{
			name: "single label",
			labels: []*metrics.Label{
				{Name: "service", Value: "svc1"},
			},
			expected: "service=svc1",
		},
		{
			name: "multiple labels sorted",
			labels: []*metrics.Label{
				{Name: "service", Value: "svc1"},
				{Name: "operation", Value: "op1"},
			},
			expected: "operation=op1,service=svc1",
		},
		{
			name: "labels with same name different values",
			labels: []*metrics.Label{
				{Name: "tag", Value: "b"},
				{Name: "tag", Value: "a"},
			},
			expected: "tag=a,tag=b",
		},
		{
			name:     "empty labels",
			labels:   []*metrics.Label{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLabelKey(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTimestampToKey tests the TimestampToKey function.
func TestTimestampToKey(t *testing.T) {
	now := time.Now()
	timestamp, _ := types.TimestampProto(now)

	key, err := TimestampToKey(timestamp)
	require.NoError(t, err)
	assert.Equal(t, now.UnixNano(), key)
}

// TestTimestampToKey_Error tests error handling in TimestampToKey.
func TestTimestampToKey_Error(t *testing.T) {
	// Create an invalid timestamp
	invalidTimestamp := &types.Timestamp{
		Seconds: -62135596801, // Before Unix epoch minimum
		Nanos:   0,
	}

	_, err := TimestampToKey(invalidTimestamp)
	assert.Error(t, err)
}

// TestCalcCallRate tests the CalcCallRate function.
func TestCalcCallRate(t *testing.T) {
	now := time.Now()
	params := metricstore.BaseQueryParameters{
		Step:    new(time.Second * 10),
		RatePer: new(time.Minute),
	}

	input := createMetricFamily("test_rate", []*metrics.Metric{
		{
			Labels: []*metrics.Label{{Name: "service", Value: "svc1"}},
			MetricPoints: []*metrics.MetricPoint{
				createMetricPoint(now.Add(-time.Minute), 100.0),
				createMetricPoint(now.Add(-50*time.Second), 150.0),
				createMetricPoint(now.Add(-40*time.Second), 200.0),
				createMetricPoint(now.Add(-30*time.Second), 250.0),
				createMetricPoint(now.Add(-20*time.Second), 300.0),
				createMetricPoint(now.Add(-10*time.Second), 350.0),
				createMetricPoint(now, 400.0),
			},
		},
	})

	result := CalcCallRate(input, params)
	assert.NotNil(t, result)
	assert.Len(t, result.Metrics, 1)
	assert.Len(t, result.Metrics[0].MetricPoints, 7)
}

// TestTrimMetricPointsBefore_EdgeCases tests edge cases for TrimMetricPointsBefore.
func TestTrimMetricPointsBefore_EdgeCases(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		input          *metrics.MetricFamily
		startMillis    int64
		expectedPoints int
	}{
		{
			name: "all points before threshold",
			input: createMetricFamily("test", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now.Add(-3*time.Minute), 10.0),
					createMetricPoint(now.Add(-2*time.Minute), 20.0),
				}),
			}),
			startMillis:    now.UnixMilli(),
			expectedPoints: 0,
		},
		{
			name: "all points after threshold",
			input: createMetricFamily("test", []*metrics.Metric{
				createMetric([]*metrics.MetricPoint{
					createMetricPoint(now, 10.0),
					createMetricPoint(now.Add(time.Minute), 20.0),
				}),
			}),
			startMillis:    now.Add(-time.Minute).UnixMilli(),
			expectedPoints: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimMetricPointsBefore(tt.input, tt.startMillis)
			assert.Len(t, result.Metrics[0].MetricPoints, tt.expectedPoints)
		})
	}
}

// TestApplySlidingWindow tests the ApplySlidingWindow function.
func TestApplySlidingWindow(t *testing.T) {
	now := time.Now()
	input := createMetricFamily("test", []*metrics.Metric{
		createMetric([]*metrics.MetricPoint{
			createMetricPoint(now.Add(-2*time.Minute), 10.0),
			createMetricPoint(now.Add(-time.Minute), 20.0),
			createMetricPoint(now, 30.0),
		}),
	})

	// Test with a simple processor that returns the sum of the window
	sumProcessor := func(_ *metrics.Metric, window []*metrics.MetricPoint) float64 {
		sum := 0.0
		for _, point := range window {
			val := point.GetGaugeValue().GetDoubleValue()
			if !math.IsNaN(val) {
				sum += val
			}
		}
		return sum
	}

	result := ApplySlidingWindow(input, 2, sumProcessor)
	assert.NotNil(t, result)
	assert.Len(t, result.Metrics, 1)
	assert.Len(t, result.Metrics[0].MetricPoints, 3)
}

// TestApplySlidingWindow_EmptyMetrics tests ApplySlidingWindow with empty metrics.
func TestApplySlidingWindow_EmptyMetrics(t *testing.T) {
	input := createMetricFamily("test", []*metrics.Metric{
		createMetric([]*metrics.MetricPoint{}),
	})

	processor := func(_ *metrics.Metric, _ []*metrics.MetricPoint) float64 {
		return 0.0
	}

	result := ApplySlidingWindow(input, 1, processor)
	assert.NotNil(t, result)
	assert.Len(t, result.Metrics, 1)
	assert.Empty(t, result.Metrics[0].MetricPoints)
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
