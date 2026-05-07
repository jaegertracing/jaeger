// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gogotypes "github.com/gogo/protobuf/types"

	openmetrics "github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// mockMetricsReader is a test double for metricsReaderInterface.
type mockMetricsReader struct {
	family *openmetrics.MetricFamily
	err    error
}

func (m *mockMetricsReader) GetLatencies(_ context.Context, _ *metricstore.LatenciesQueryParameters) (*openmetrics.MetricFamily, error) {
	return m.family, m.err
}

func (m *mockMetricsReader) GetCallRates(_ context.Context, _ *metricstore.CallRateQueryParameters) (*openmetrics.MetricFamily, error) {
	return m.family, m.err
}

func (m *mockMetricsReader) GetErrorRates(_ context.Context, _ *metricstore.ErrorRateQueryParameters) (*openmetrics.MetricFamily, error) {
	return m.family, m.err
}

// makeTimestamp builds a gogo protobuf Timestamp from a time.Time.
func makeTimestamp(t time.Time) *gogotypes.Timestamp {
	return &gogotypes.Timestamp{
		Seconds: t.Unix(),
		Nanos:   int32(t.Nanosecond()),
	}
}

// makeMetricFamily builds a minimal MetricFamily with one metric and one data point.
func makeMetricFamily(serviceName string, val float64, ts time.Time) *openmetrics.MetricFamily {
	return &openmetrics.MetricFamily{
		Metrics: []*openmetrics.Metric{
			{
				Labels: []*openmetrics.Label{
					{Name: "service_name", Value: serviceName},
					{Name: "span_kind", Value: "SERVER"},
				},
				MetricPoints: []*openmetrics.MetricPoint{
					{
						Value: &openmetrics.MetricPoint_GaugeValue{
							GaugeValue: &openmetrics.GaugeValue{
								Value: &openmetrics.GaugeValue_DoubleValue{
									DoubleValue: val,
								},
							},
						},
						Timestamp: makeTimestamp(ts),
					},
				},
			},
		},
	}
}

// --- NewGetServiceMetricsHandler ---

func TestNewGetServiceMetricsHandler(t *testing.T) {
	reader := &mockMetricsReader{}
	handler := NewGetServiceMetricsHandler(reader)
	assert.NotNil(t, handler)
}

// --- Validation errors ---

func TestGetServiceMetricsHandler_EmptyServices(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		MetricType: "latency",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "services must not be empty")
}

func TestGetServiceMetricsHandler_EmptyMetricType(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services: []string{"frontend"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metric_type must be one of")
}

func TestGetServiceMetricsHandler_UnknownMetricType(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "p99_latency",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown metric_type")
}

func TestGetServiceMetricsHandler_InvalidSpanKind(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "latency",
		SpanKinds:  []string{"INVALID"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown span kind")
}

// --- Disabled metrics storage ---

func TestGetServiceMetricsHandler_DisabledStorage(t *testing.T) {
	h := &getServiceMetricsHandler{
		reader: &mockMetricsReader{err: disabled.ErrDisabled},
	}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "latency",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics storage is not configured")
}

// --- Storage error ---

func TestGetServiceMetricsHandler_StorageError(t *testing.T) {
	h := &getServiceMetricsHandler{
		reader: &mockMetricsReader{err: errors.New("prometheus unavailable")},
	}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "call_rate",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics query failed")
}

// --- Successful queries for each metric type ---

func TestGetServiceMetricsHandler_Latency_Success(t *testing.T) {
	ts := time.Now().Truncate(time.Millisecond)
	family := makeMetricFamily("frontend", 0.042, ts)
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: family}}

	_, output, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "latency",
		Quantile:   0.99,
	})
	require.NoError(t, err)
	assert.Equal(t, "latency", output.MetricType)
	require.Len(t, output.Metrics, 1)
	assert.Equal(t, "frontend", output.Metrics[0].ServiceName)
	assert.Equal(t, "SERVER", output.Metrics[0].SpanKind)
	require.Len(t, output.Metrics[0].DataPoints, 1)
	assert.InDelta(t, 0.042, output.Metrics[0].DataPoints[0].Value, 1e-9)
	assert.InDelta(t, ts.UnixMilli(), output.Metrics[0].DataPoints[0].TimestampMs, 1000)
}

func TestGetServiceMetricsHandler_CallRate_Success(t *testing.T) {
	ts := time.Now().Truncate(time.Millisecond)
	family := makeMetricFamily("backend", 12.5, ts)
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: family}}

	_, output, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"backend"},
		MetricType: "call_rate",
	})
	require.NoError(t, err)
	assert.Equal(t, "call_rate", output.MetricType)
	require.Len(t, output.Metrics, 1)
	assert.Equal(t, "backend", output.Metrics[0].ServiceName)
	assert.InDelta(t, 12.5, output.Metrics[0].DataPoints[0].Value, 1e-9)
}

func TestGetServiceMetricsHandler_ErrorRate_Success(t *testing.T) {
	ts := time.Now().Truncate(time.Millisecond)
	family := makeMetricFamily("checkout", 0.01, ts)
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: family}}

	_, output, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"checkout"},
		MetricType: "error_rate",
	})
	require.NoError(t, err)
	assert.Equal(t, "error_rate", output.MetricType)
	require.Len(t, output.Metrics, 1)
	assert.Equal(t, "checkout", output.Metrics[0].ServiceName)
}

// --- Default parameter handling ---

func TestGetServiceMetricsHandler_DefaultQuantile(t *testing.T) {
	// Quantile 0 should default to 0.95 — handler must not error
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: makeMetricFamily("svc", 1.0, time.Now())}}
	_, output, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"svc"},
		MetricType: "latency",
		Quantile:   0, // invalid → should default to 0.95
	})
	require.NoError(t, err)
	assert.Equal(t, "latency", output.MetricType)
}

func TestGetServiceMetricsHandler_DefaultEndTime(t *testing.T) {
	// Empty EndTime should default to now without error
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: makeMetricFamily("svc", 1.0, time.Now())}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"svc"},
		MetricType: "call_rate",
		EndTime:    "", // should default to time.Now()
	})
	require.NoError(t, err)
}

func TestGetServiceMetricsHandler_CustomDurations(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: makeMetricFamily("svc", 1.0, time.Now())}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"svc"},
		MetricType: "error_rate",
		Lookback:   "30m",
		Step:       "5m",
		RatePer:    "2m",
	})
	require.NoError(t, err)
}

// --- GroupByOperation and SpanKinds ---

func TestGetServiceMetricsHandler_GroupByOperation(t *testing.T) {
	ts := time.Now()
	family := &openmetrics.MetricFamily{
		Metrics: []*openmetrics.Metric{
			{
				Labels: []*openmetrics.Label{
					{Name: "service_name", Value: "frontend"},
					{Name: "operation", Value: "/checkout"},
					{Name: "span_kind", Value: "SERVER"},
				},
				MetricPoints: []*openmetrics.MetricPoint{
					{
						Value: &openmetrics.MetricPoint_GaugeValue{
							GaugeValue: &openmetrics.GaugeValue{
								Value: &openmetrics.GaugeValue_DoubleValue{DoubleValue: 0.05},
							},
						},
						Timestamp: makeTimestamp(ts),
					},
				},
			},
		},
	}
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: family}}

	_, output, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:         []string{"frontend"},
		MetricType:       "latency",
		GroupByOperation: true,
	})
	require.NoError(t, err)
	require.Len(t, output.Metrics, 1)
	assert.Equal(t, "/checkout", output.Metrics[0].OperationName)
}

func TestGetServiceMetricsHandler_ValidSpanKinds(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: makeMetricFamily("svc", 1.0, time.Now())}}
	_, _, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"svc"},
		MetricType: "latency",
		SpanKinds:  []string{"SERVER", "CLIENT"},
	})
	require.NoError(t, err)
}

// --- Nil / empty family ---

func TestGetServiceMetricsHandler_NilFamily(t *testing.T) {
	h := &getServiceMetricsHandler{reader: &mockMetricsReader{family: nil}}
	_, output, err := h.handle(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "latency",
	})
	require.NoError(t, err)
	assert.Equal(t, "latency", output.MetricType)
	assert.Empty(t, output.Metrics)
}

// --- validateSpanKinds unit tests ---

func TestValidateSpanKinds_Valid(t *testing.T) {
	kinds := []string{"SERVER", "CLIENT", "PRODUCER", "CONSUMER", "INTERNAL"}
	assert.NoError(t, validateSpanKinds(kinds))
}

func TestValidateSpanKinds_Empty(t *testing.T) {
	assert.NoError(t, validateSpanKinds(nil))
	assert.NoError(t, validateSpanKinds([]string{}))
}

func TestValidateSpanKinds_Invalid(t *testing.T) {
	err := validateSpanKinds([]string{"UNKNOWN"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown span kind")
}

// --- parseDurationOrDefault unit tests ---

func TestParseDurationOrDefault_Valid(t *testing.T) {
	assert.Equal(t, 30*time.Minute, parseDurationOrDefault("30m", time.Hour))
}

func TestParseDurationOrDefault_Empty(t *testing.T) {
	assert.Equal(t, time.Hour, parseDurationOrDefault("", time.Hour))
}

func TestParseDurationOrDefault_Invalid(t *testing.T) {
	assert.Equal(t, time.Hour, parseDurationOrDefault("notaduration", time.Hour))
}
