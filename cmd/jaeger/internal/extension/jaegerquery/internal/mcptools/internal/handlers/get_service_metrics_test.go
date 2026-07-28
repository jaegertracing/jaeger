// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"math"
	"testing"
	"time"

	gogotypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	metricstoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/mocks"
)

func makeTestMetricFamily() *metrics.MetricFamily {
	return &metrics.MetricFamily{
		Name: "service_latencies",
		Help: "0.95th quantile latency, grouped by service, in milliseconds",
		Metrics: []*metrics.Metric{
			nil, // a nil metric entry must be skipped, not panic
			{
				Labels: []*metrics.Label{
					{Name: "service_name", Value: "frontend"},
					{Name: "operation", Value: "GET /dispatch"},
					{Name: "span_kind", Value: "SPAN_KIND_SERVER"},
				},
				MetricPoints: []*metrics.MetricPoint{
					nil, // a nil point must be skipped, not panic
					{
						Value:     &metrics.MetricPoint_GaugeValue{GaugeValue: &metrics.GaugeValue{Value: &metrics.GaugeValue_DoubleValue{DoubleValue: 42.5}}},
						Timestamp: &gogotypes.Timestamp{Seconds: 1700000000, Nanos: 500_000_000},
					},
					{
						// NaN is what the metricstore reports when the backend
						// has no data for this timestamp.
						Value:     &metrics.MetricPoint_GaugeValue{GaugeValue: &metrics.GaugeValue{Value: &metrics.GaugeValue_DoubleValue{DoubleValue: math.NaN()}}},
						Timestamp: &gogotypes.Timestamp{Seconds: 1700000060},
					},
					{
						// +/-Inf can come out of quantile calculations and is
						// not JSON-encodable either.
						Value:     &metrics.MetricPoint_GaugeValue{GaugeValue: &metrics.GaugeValue{Value: &metrics.GaugeValue_DoubleValue{DoubleValue: math.Inf(1)}}},
						Timestamp: &gogotypes.Timestamp{Seconds: 1700000120},
					},
				},
			},
		},
	}
}

func TestGetServiceMetrics_Latency(t *testing.T) {
	reader := metricstoremocks.NewReader(t)
	reader.On("GetLatencies", mock.Anything, mock.MatchedBy(func(p *metricstore.LatenciesQueryParameters) bool {
		return p.Quantile == 0.95 && // default applied
			len(p.ServiceNames) == 1 && p.ServiceNames[0] == "frontend" &&
			*p.Lookback == time.Hour && *p.Step == time.Minute && *p.RatePer == 10*time.Minute &&
			len(p.SpanKinds) == 1 && p.SpanKinds[0] == "SPAN_KIND_SERVER"
	})).Return(makeTestMetricFamily(), nil).Once()

	handler := NewGetServiceMetricsHandler(reader)
	_, output, err := handler(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "latency",
	})
	require.NoError(t, err)

	assert.Equal(t, "latency", output.MetricType)
	assert.InDelta(t, 0.95, output.Quantile, 1e-9)
	assert.Equal(t, "service_latencies", output.MetricName)
	assert.Equal(t, "0.95th quantile latency, grouped by service, in milliseconds", output.Description)
	require.Len(t, output.Metrics, 1)

	series := output.Metrics[0]
	assert.Equal(t, "frontend", series.ServiceName)
	assert.Equal(t, "GET /dispatch", series.OperationName)
	assert.Equal(t, "SPAN_KIND_SERVER", series.SpanKind)
	require.Len(t, series.DataPoints, 3)
	assert.Equal(t, int64(1700000000500), series.DataPoints[0].TimestampMs)
	require.NotNil(t, series.DataPoints[0].Value)
	assert.InDelta(t, 42.5, *series.DataPoints[0].Value, 1e-9)
	assert.Equal(t, int64(1700000060000), series.DataPoints[1].TimestampMs)
	assert.Nil(t, series.DataPoints[1].Value, "NaN from the backend must become null, not a fake number")
	assert.Equal(t, int64(1700000120000), series.DataPoints[2].TimestampMs)
	assert.Nil(t, series.DataPoints[2].Value, "Inf from the backend must become null; json.Marshal cannot encode it")
}

func TestGetServiceMetrics_CallRateAndErrorRate(t *testing.T) {
	tests := []struct {
		metricType string
		mockMethod string
	}{
		{metricType: "call_rate", mockMethod: "GetCallRates"},
		{metricType: "error_rate", mockMethod: "GetErrorRates"},
	}
	for _, test := range tests {
		t.Run(test.metricType, func(t *testing.T) {
			reader := metricstoremocks.NewReader(t)
			reader.On(test.mockMethod, mock.Anything, mock.Anything).Return(makeTestMetricFamily(), nil).Once()

			handler := NewGetServiceMetricsHandler(reader)
			_, output, err := handler(context.Background(), nil, types.GetServiceMetricsInput{
				Services:   []string{"frontend"},
				MetricType: test.metricType,
			})
			require.NoError(t, err)
			assert.Equal(t, test.metricType, output.MetricType)
			assert.Zero(t, output.Quantile, "quantile applies to latency only")
			require.Len(t, output.Metrics, 1)
		})
	}
}

func TestGetServiceMetrics_CustomParameters(t *testing.T) {
	endTime := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	reader := metricstoremocks.NewReader(t)
	reader.On("GetLatencies", mock.Anything, mock.MatchedBy(func(p *metricstore.LatenciesQueryParameters) bool {
		return p.Quantile == 0.99 &&
			p.EndTime.Equal(endTime) &&
			*p.Lookback == 30*time.Minute && *p.Step == 5*time.Second && *p.RatePer == time.Minute &&
			p.GroupByOperation &&
			len(p.SpanKinds) == 2 && p.SpanKinds[0] == "SPAN_KIND_CLIENT" && p.SpanKinds[1] == "SPAN_KIND_PRODUCER"
	})).Return(&metrics.MetricFamily{}, nil).Once()

	handler := NewGetServiceMetricsHandler(reader)
	_, output, err := handler(context.Background(), nil, types.GetServiceMetricsInput{
		Services:         []string{"frontend"},
		MetricType:       "latency",
		Quantile:         0.99,
		EndTime:          "2026-07-28T10:00:00Z",
		Lookback:         "30m",
		Step:             "5s",
		RatePer:          "1m",
		GroupByOperation: true,
		SpanKinds:        []string{"CLIENT", "producer"},
	})
	require.NoError(t, err)
	assert.NotNil(t, output.Metrics, "empty result must be an empty list, not null")
	assert.Empty(t, output.Metrics)
}

func TestGetServiceMetrics_InputValidation(t *testing.T) {
	tests := []struct {
		name        string
		input       types.GetServiceMetricsInput
		expectedErr string
	}{
		{
			name:        "missing services",
			input:       types.GetServiceMetricsInput{MetricType: "latency"},
			expectedErr: "at least one service is required",
		},
		{
			name:        "invalid metric type",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "throughput"},
			expectedErr: "invalid metric_type",
		},
		{
			name:        "invalid quantile",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", Quantile: 1.5},
			expectedErr: "invalid quantile",
		},
		{
			name:        "invalid end time",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", EndTime: "yesterday"},
			expectedErr: "invalid end_time",
		},
		{
			name:        "negative lookback",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", Lookback: "-1h"},
			expectedErr: "invalid lookback",
		},
		{
			name:        "malformed step",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", Step: "fast"},
			expectedErr: "invalid step",
		},
		{
			name:        "negative rate_per",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", RatePer: "-1m"},
			expectedErr: "invalid rate_per",
		},
		{
			name:        "invalid span kind",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", SpanKinds: []string{"sideways"}},
			expectedErr: "invalid span kind",
		},
		{
			name:        "too many data points",
			input:       types.GetServiceMetricsInput{Services: []string{"a"}, MetricType: "latency", Lookback: "48h", Step: "1m"},
			expectedErr: "data points per series",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewGetServiceMetricsHandler(&metricstoremocks.Reader{})
			_, _, err := handler(context.Background(), nil, test.input)
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func TestGetServiceMetrics_ReaderError(t *testing.T) {
	reader := metricstoremocks.NewReader(t)
	reader.On("GetCallRates", mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()

	handler := NewGetServiceMetricsHandler(reader)
	_, _, err := handler(context.Background(), nil, types.GetServiceMetricsInput{
		Services:   []string{"frontend"},
		MetricType: "call_rate",
	})
	require.ErrorContains(t, err, "failed to get call_rate metrics")
}
