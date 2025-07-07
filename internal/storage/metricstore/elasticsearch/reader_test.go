// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	esmetrics "github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

var mockCallRateQuery = `{
  "query": {
    "bool": {
      "filter": [
        {"terms": {"process.serviceName": ["driver"]}},
        {"terms": {"tag.span@kind": ["server"]}},
        {"range": {
          "startTimeMillis": {
            "gte": 1749894300000,
            "lte": 1749894960000,
            "format": "epoch_millis"
          }
        }}
      ]
    }
  },
  "size": 0,
  "aggregations": {
    "results_buckets": {
      "date_histogram": {
        "field": "startTimeMillis",
        "fixed_interval": "60000ms",
        "min_doc_count": 0,
        "extended_bounds": {
          "min": 1749894900000,
          "max": 1749894960000
        }
      },
      "aggregations": {
        "cumulative_requests": {
          "cumulative_sum": {
            "buckets_path": "_count"}}}}}}
`

var mockLatencyQuery = `
{
  "size": 0,
  "query": {
    "bool": {
      "filter": [
		{"terms": {"process.serviceName": ["driver"]}},
        {"terms": {"tag.span@kind": ["server"]}},
        {"range": {
            "startTimeMillis": {
				"gte": 1749894300000,
				"lte": 1749894960000,
				"format": "epoch_millis"
			}}}]}},
  "aggs": {
    "requests_per_bucket": {
      "date_histogram": {
        "extended_bounds": {
          "min": 1749894900000,
          "max": 1749894960000
        },
        "field": "startTimeMillis",
        "fixed_interval": "60000ms",
        "min_doc_count": 0
      },
      "aggs": {
        "percentiles_of_bucket": {
          "percentiles": {
            "field": "duration",
            "percents": [95]}}}}}}
`

var mockErrorRateQuery = `{
  "query": {
    "bool": {
      "filter": [
        {"terms": {"process.serviceName": ["driver"]}},
        {"terms": {"tag.span@kind": ["server"]}},
		{"term": {"tag.error": true}},
        {"range": {
          "startTimeMillis": {
            "gte": 1749894300000,
            "lte": 1749894960000,
            "format": "epoch_millis"
          }
        }}
      ]
    }
  },
  "size": 0,
  "aggregations": {
    "results_buckets": {
      "date_histogram": {
        "field": "startTimeMillis",
        "fixed_interval": "60000ms",
        "min_doc_count": 0,
        "extended_bounds": {
          "min": 1749894900000,
          "max": 1749894960000
        }
      },
      "aggregations": {
        "cumulative_requests": {
          "cumulative_sum": {
            "buckets_path": "_count"}}}}}}
`

const (
	mockEsValidResponse           = "testdata/output_valid_es.json"
	mockCallRateResponse          = "testdata/output_call_rate.json"
	mockCallRateOperationResponse = "testdata/output_call_rate_operation.json"
	mockEmptyResponse             = "testdata/output_empty.json"
	mockErrorResponse             = "testdata/output_error_es.json"
	mockLatencyResponse           = "testdata/output_latencies.json" // simple case
	mockLatencyOperationResponse  = "testdata/output_latencies_operation.json"
	mockErrorRateResponse         = "testdata/output_errors_rate.json"
	mockErrRateOperationResponse  = "testdata/output_errors_rate_operation.json"
)

type metricsTestCase struct {
	name         string
	serviceNames []string
	spanKinds    []string
	groupByOp    bool
	query        string // Elasticsearch query to validate
	responseFile string
	wantName     string
	wantDesc     string
	wantLabels   map[string]string
	wantPoints   []struct {
		TimestampSec int64
		Value        float64
	}
	wantErr string
}

func tracerProvider(t *testing.T) (trace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() {
		require.NoError(t, tp.ForceFlush(context.Background()))
		require.NoError(t, tp.Shutdown(context.Background()))
	})
	return tp, exporter
}

func clientProvider(t *testing.T, c *config.Configuration, logger *zap.Logger, metricsFactory esmetrics.Factory) es.Client {
	client, err := config.NewClient(context.Background(), c, logger, metricsFactory)
	require.NoError(t, err)
	require.NotNil(t, client)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}

func assertMetricFamily(t *testing.T, got *metrics.MetricFamily, m metricsTestCase) {
	if got == nil {
		t.Fatal("Expected non-nil MetricFamily")
	}
	assert.Equal(t, m.wantName, got.Name, "Metric name mismatch")
	assert.Equal(t, m.wantDesc, got.Help, "Metric description mismatch")
	assert.Equal(t, metrics.MetricType_GAUGE, got.Type, "Metric type mismatch")
	assert.Len(t, got.Metrics, 1, "Expected one metric")

	metric := got.Metrics[0]
	gotLabels := make(map[string]string)
	for _, label := range metric.Labels {
		gotLabels[label.Name] = label.Value
	}
	assert.Equal(t, m.wantLabels, gotLabels, "Labels mismatch")

	if len(m.wantPoints) == 0 {
		assert.Empty(t, metric.MetricPoints, "Expected no metric points")
		return
	}

	assert.Len(t, metric.MetricPoints, len(m.wantPoints), "Metric points count mismatch")
	for i, point := range metric.MetricPoints {
		assert.Equal(t, m.wantPoints[i].TimestampSec, point.Timestamp.GetSeconds(), "Timestamp mismatch for point %d", i)
		actualValue := point.Value.(*metrics.MetricPoint_GaugeValue).GaugeValue.GetDoubleValue()
		assert.InDelta(t, m.wantPoints[i].Value, actualValue, 0.01, "Value mismatch for point %d", i)
	}
}

func TestScaleToMillisAndRound_EmptyWindow(t *testing.T) {
	var window []*metrics.MetricPoint
	result := scaleToMillisAndRound(window)
	assert.True(t, math.IsNaN(result))
}

func Test_ErrorCases(t *testing.T) {
	endTime := time.UnixMilli(0)
	tests := []struct {
		name    string
		params  metricstore.BaseQueryParameters
		wantErr string
	}{
		{
			name:    "nil base params",
			wantErr: "invalid parameters",
		},
		{
			name:    "nil end time params",
			params:  metricstore.BaseQueryParameters{},
			wantErr: "invalid parameters",
		},
		{
			name: "nil step params",
			params: metricstore.BaseQueryParameters{
				EndTime: &(endTime),
			},
			wantErr: "invalid parameters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := startMockEsServer(t, "", mockEmptyResponse)
			defer mockServer.Close()
			reader, _ := setupMetricsReaderFromServer(t, mockServer)
			callRateMetricFamily, err := reader.GetCallRates(context.Background(), &metricstore.CallRateQueryParameters{BaseQueryParameters: tc.params})
			helperAssertError(t, err, tc.wantErr, callRateMetricFamily)
			latenciesMetricFamily, err := reader.GetLatencies(context.Background(), &metricstore.LatenciesQueryParameters{BaseQueryParameters: tc.params})
			helperAssertError(t, err, tc.wantErr, latenciesMetricFamily)
		})
	}
}

func helperAssertError(t *testing.T, err error, wantErr string, result *metrics.MetricFamily) {
	require.Error(t, err)
	assert.Contains(t, err.Error(), wantErr)
	require.Nil(t, result)
}

func TestGetCallRates(t *testing.T) {
	expectedPoints := []struct {
		TimestampSec int64
		Value        float64
	}{
		{1749894840, math.NaN()},
		{1749894900, math.NaN()},
		{1749894960, math.NaN()},
		{1749895020, math.NaN()},
		{1749895080, math.NaN()},
		{1749895140, math.NaN()},
		{1749895200, math.NaN()},
		{1749895260, math.NaN()},
		{1749895320, math.NaN()},
		{1749895380, 0.75},
		{1749895440, 0.9},
		{1749895500, math.NaN()},
	}
	tests := []metricsTestCase{
		{
			name:         "group by service only",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			query:        mockCallRateQuery,
			responseFile: mockCallRateResponse,
			wantName:     "service_call_rate",
			wantDesc:     "calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: expectedPoints,
		},
		{
			name:         "group by service and operation",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    true,
			responseFile: mockCallRateOperationResponse,
			wantName:     "service_operation_call_rate",
			wantDesc:     "calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"service_name": "driver",
				"operation":    "/FindNearest",
			},
			wantPoints: expectedPoints,
		},
		{
			name:         "different service names",
			serviceNames: []string{"jaeger"},
			spanKinds:    []string{"SPAN_KIND_SERVER", "SPAN_KIND_CLIENT"},
			groupByOp:    false,
			responseFile: mockCallRateResponse,
			wantName:     "service_call_rate",
			wantDesc:     "calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "jaeger",
			},
			wantPoints: expectedPoints,
		},
		{
			name:         "empty response",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: mockEmptyResponse,
			wantName:     "service_call_rate",
			wantDesc:     "calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: nil,
		},
		{
			name:         "server error",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: mockErrorResponse,
			wantErr:      "failed executing metrics query",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := startMockEsServer(t, tc.query, tc.responseFile)
			defer mockServer.Close()
			reader, exporter := setupMetricsReaderFromServer(t, mockServer)

			params := &metricstore.CallRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParameters(tc),
			}

			metricFamily, err := reader.GetCallRates(context.Background(), params)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, metricFamily)
			} else {
				require.NoError(t, err)
				assertMetricFamily(t, metricFamily, tc)
			}

			spans := exporter.GetSpans()
			if tc.wantErr == "" {
				assert.Len(t, spans, 1, "Expected one span for the Elasticsearch query")
			}
		})
	}
}

func TestGetLatencies(t *testing.T) {
	tests := []metricsTestCase{
		{
			name:         "group by service only",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			query:        mockLatencyQuery,
			responseFile: mockLatencyResponse,
			wantName:     "service_latencies",
			wantDesc:     "0.95th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: []struct {
				TimestampSec int64
				Value        float64
			}{
				{1749894900, 0.2},
				{1749894960, 0.21},
				{1749895020, math.NaN()},
			},
		},
		{
			name:         "group by service and operation",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    true,
			responseFile: mockLatencyOperationResponse,
			wantName:     "service_operation_latencies",
			wantDesc:     "0.95th quantile latency, grouped by service & operation",
			wantLabels: map[string]string{
				"service_name": "driver",
				"operation":    "/FindNearest",
			},
			wantPoints: []struct {
				TimestampSec int64
				Value        float64
			}{
				{1749894900, 0.2},
				{1749894960, 0.21},
			},
		},
		{
			name:         "empty response",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: mockEmptyResponse,
			wantName:     "service_latencies",
			wantDesc:     "0.95th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: nil,
		},
		{
			name:         "server error",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: mockErrorResponse,
			wantErr:      "failed executing metrics query",
		},
		{
			name:         "convert error",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    true,
			responseFile: "testdata/output_error_latencies.json",
			wantErr:      "failed to convert aggregations to metrics",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := startMockEsServer(t, tc.query, tc.responseFile)
			defer mockServer.Close()
			reader, exporter := setupMetricsReaderFromServer(t, mockServer)

			params := &metricstore.LatenciesQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParameters(tc),
				Quantile:            0.95,
			}

			metricFamily, err := reader.GetLatencies(context.Background(), params)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, metricFamily)
			} else {
				require.NoError(t, err)
				assertMetricFamily(t, metricFamily, tc)
			}

			spans := exporter.GetSpans()
			if tc.wantErr == "" {
				assert.Len(t, spans, 1, "Expected one span for the Elasticsearch query")
			}
		})
	}
}

func TestGetLatencies_WithDifferentQuantiles(t *testing.T) {
	tests := []metricsTestCase{
		{
			name:         "0.5 quantile",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: "testdata/output_latencies_50.json",
			wantName:     "service_latencies",
			wantDesc:     "0.50th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: []struct {
				TimestampSec int64
				Value        float64
			}{
				{1749894840, math.NaN()},
				{1749894900, 0.15},
				{1749894960, 0.16},
				{1749895020, 0.17},
				{1749895080, 0.18},
				{1749895140, 0.19},
				{1749895200, math.NaN()},
				{1749895260, 0.2},
				{1749895320, 0.21},
				{1749895380, 0.22},
				{1749895440, 0.23},
			},
		},
		{
			name:         "0.75 quantile",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: "testdata/output_latencies_75.json",
			wantName:     "service_latencies",
			wantDesc:     "0.75th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: []struct {
				TimestampSec int64
				Value        float64
			}{
				{1749894840, math.NaN()},
				{1749894900, 0.25},
				{1749894960, 0.26},
				{1749895020, 0.27},
				{1749895080, 0.28},
				{1749895140, 0.29},
				{1749895200, math.NaN()},
				{1749895260, 0.3},
				{1749895320, 0.31},
				{1749895380, 0.32},
				{1749895440, 0.33},
			},
		},
		{
			name:         "0.95 quantile",
			serviceNames: []string{"driver"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
			groupByOp:    false,
			responseFile: "testdata/output_latencies_95.json",
			wantName:     "service_latencies",
			wantDesc:     "0.95th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "driver",
			},
			wantPoints: []struct {
				TimestampSec int64
				Value        float64
			}{
				{1749894840, math.NaN()},
				{1749894900, 0.45},
				{1749894960, 0.46},
				{1749895020, 0.47},
				{1749895080, 0.48},
				{1749895140, 0.49},
				{1749895200, math.NaN()},
				{1749895260, 0.50},
				{1749895320, 0.51},
				{1749895380, 0.52},
				{1749895440, 0.53},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := startMockEsServer(t, "", tc.responseFile)
			defer mockServer.Close()
			reader, exporter := setupMetricsReaderFromServer(t, mockServer)

			params := &metricstore.LatenciesQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParameters(tc),
				Quantile:            0.95, // Will be adjusted based on test case
			}

			// Set the correct quantile for each test case
			switch tc.name {
			case "0.5 quantile":
				params.Quantile = 0.5
			case "0.75 quantile":
				params.Quantile = 0.75
			case "0.95 quantile":
				params.Quantile = 0.95
			}

			metricFamily, err := reader.GetLatencies(context.Background(), params)
			require.NoError(t, err)
			assertMetricFamily(t, metricFamily, tc)

			spans := exporter.GetSpans()
			assert.Len(t, spans, 1, "Expected one span for the Elasticsearch query")
		})
	}
}

func TestGetLatenciesBucketsToPoints_ErrorCases(t *testing.T) {
	tests := []struct {
		name            string
		buckets         []*elastic.AggregationBucketHistogramItem
		percentileValue float64
	}{
		{
			name:            "missing percentiles aggregation",
			percentileValue: 95.0,
			buckets: []*elastic.AggregationBucketHistogramItem{
				{
					Key:          1749894900000,
					DocCount:     1,
					Aggregations: map[string]json.RawMessage{},
				},
			},
		},
		{
			name:            "missing percentile key",
			percentileValue: 95.0,
			buckets: []*elastic.AggregationBucketHistogramItem{
				{
					Key:      1749894900000,
					DocCount: 1,
					Aggregations: map[string]json.RawMessage{
						percentilesAggName: json.RawMessage(`{"values": {"90.0": 200.0}}`),
					},
				},
			},
		},
		{
			name: "nil percentile value",
			buckets: []*elastic.AggregationBucketHistogramItem{
				{
					Key:      1749894900000,
					DocCount: 1,
					Aggregations: map[string]json.RawMessage{
						percentilesAggName: json.RawMessage(`{"values": {"95.0": null}}`),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bucketsToLatencies(tt.buckets, tt.percentileValue)
			assert.True(t, math.IsNaN(result[0].Value))
		})
	}
}

func TestGetErrorRates(t *testing.T) {
	expectedPoints := []struct {
		TimestampSec int64
		Value        float64
	}{
		{1749894840, math.NaN()},
		{1749894900, math.NaN()},
		{1749894960, math.NaN()},
		{1749895020, math.NaN()},
		{1749895080, math.NaN()},
		{1749895140, math.NaN()},
		{1749895200, math.NaN()},
		{1749895260, math.NaN()},
		{1749895320, math.NaN()},
		{1749895380, 0.5},
		{1749895440, 0.75},
		{1749895500, math.NaN()},
	}

	tests := []struct {
		metricsTestCase
		callRateFile string
	}{
		{
			metricsTestCase: metricsTestCase{
				name:         "group by service only - successful",
				serviceNames: []string{"driver"},
				spanKinds:    []string{"SPAN_KIND_SERVER"},
				groupByOp:    false,
				query:        mockErrorRateQuery,
				responseFile: mockErrorRateResponse,
				wantName:     "service_error_rate",
				wantDesc:     "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
				wantLabels: map[string]string{
					"service_name": "driver",
				},
				wantPoints: expectedPoints,
			},
			callRateFile: mockCallRateResponse,
		},
		{
			metricsTestCase: metricsTestCase{
				name:         "group by service and operation - successful",
				serviceNames: []string{"driver"},
				spanKinds:    []string{"SPAN_KIND_SERVER"},
				groupByOp:    true,
				responseFile: mockErrRateOperationResponse,
				wantName:     "service_operation_error_rate",
				wantDesc:     "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service & operation",
				wantLabels: map[string]string{
					"service_name": "driver",
					"operation":    "/FindNearest",
				},
				wantPoints: expectedPoints,
			},
			callRateFile: mockCallRateOperationResponse,
		},
		{
			metricsTestCase: metricsTestCase{
				name:         "empty error response",
				serviceNames: []string{"driver"},
				spanKinds:    []string{"SPAN_KIND_SERVER"},
				groupByOp:    false,
				responseFile: mockEmptyResponse,
				wantName:     "service_error_rate",
				wantDesc:     "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
				wantLabels: map[string]string{
					"service_name": "driver",
				},
				wantPoints: nil,
			},
			callRateFile: mockCallRateResponse,
		},
		{
			metricsTestCase: metricsTestCase{
				name:         "empty call rate response",
				serviceNames: []string{"driver"},
				spanKinds:    []string{"SPAN_KIND_SERVER"},
				groupByOp:    false,
				responseFile: mockErrorRateResponse,
				wantName:     "service_error_rate",
				wantDesc:     "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
				wantLabels: map[string]string{
					"service_name": "driver",
				},
				wantPoints: nil,
			},
			callRateFile: mockEmptyResponse,
		},
		{
			metricsTestCase: metricsTestCase{
				name:         "error query fails",
				serviceNames: []string{"driver"},
				spanKinds:    []string{"SPAN_KIND_SERVER"},
				groupByOp:    false,
				responseFile: mockErrorResponse,
				wantErr:      "failed executing metrics query",
			},
			callRateFile: mockCallRateResponse,
		},
		{
			metricsTestCase: metricsTestCase{
				name:         "call rate query fails",
				serviceNames: []string{"driver"},
				spanKinds:    []string{"SPAN_KIND_SERVER"},
				groupByOp:    false,
				responseFile: mockErrorRateResponse,
				wantErr:      "failed executing metrics query",
			},
			callRateFile: mockErrorResponse,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := startMockEsErrorRateServer(t, tc.query, tc.responseFile, tc.callRateFile)
			defer mockServer.Close()
			reader, exporter := setupMetricsReaderFromServer(t, mockServer)
			params := &metricstore.ErrorRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParameters(tc.metricsTestCase),
			}

			metricFamily, err := reader.GetErrorRates(context.Background(), params)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, metricFamily)
			} else {
				require.NoError(t, err)
				assertMetricFamily(t, metricFamily, metricsTestCase{
					wantName:   tc.wantName,
					wantDesc:   tc.wantDesc,
					wantLabels: tc.wantLabels,
					wantPoints: tc.wantPoints,
				})
			}

			spans := exporter.GetSpans()
			if tc.wantErr == "" {
				assert.GreaterOrEqual(t, len(spans), 1, "Expected at least one span for the Elasticsearch queries")
			}
		})
	}
}

func TestGetMinStepDuration(t *testing.T) {
	mockServer := startMockEsServer(t, "", mockEsValidResponse)
	defer mockServer.Close()
	reader, _ := setupMetricsReaderFromServer(t, mockServer)
	minStep, err := reader.GetMinStepDuration(context.Background(), &metricstore.MinStepDurationQueryParameters{})
	require.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}

func TestGetCallRateBucketsToPoints_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		buckets []*elastic.AggregationBucketHistogramItem
	}{
		{
			name: "nil cumulative sum value",
			buckets: []*elastic.AggregationBucketHistogramItem{
				{
					Key:      1749894900000,
					DocCount: 1,
					Aggregations: map[string]json.RawMessage{
						culmuAggName: json.RawMessage(`{"value": null}`),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bucketsToCallRate(tt.buckets)
			assert.True(t, math.IsNaN(result[0].Value))
		})
	}
}

func isErrorQuery(query map[string]any) bool {
	if q, ok := query["query"].(map[string]any); ok {
		if b, ok := q["bool"].(map[string]any); ok {
			if filters, ok := b["filter"].([]any); ok {
				for _, f := range filters {
					if term, ok := f.(map[string]any); ok {
						if _, ok := term["term"].(map[string]any); ok {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func sendResponse(t *testing.T, w http.ResponseWriter, responseFile string) {
	bytes, err := os.ReadFile(responseFile)
	require.NoError(t, err)

	_, err = w.Write(bytes)
	require.NoError(t, err)
}

func startMockEsErrorRateServer(t *testing.T, wantEsQuery string, responseFile string, callRateResponseFile string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Handle initial ping request
		if r.Method == http.MethodHead || r.URL.Path == "/" {
			sendResponse(t, w, mockEsValidResponse)
			return
		}

		// Read request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err, "Failed to read request body")
		defer r.Body.Close()

		// Determine which response to return based on query content
		var query map[string]any
		json.Unmarshal(body, &query)

		// Check if this is an error query (contains error term filter)
		if isErrorQuery(query) {
			// Validate query if provided
			checkQuery(t, wantEsQuery, body)
			sendResponse(t, w, responseFile)
		} else {
			sendResponse(t, w, callRateResponseFile)
		}
	}))
}

func startMockEsServer(t *testing.T, wantEsQuery string, responseFile string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Handle initial ping request
		if r.Method == http.MethodHead || r.URL.Path == "/" {
			sendResponse(t, w, mockEsValidResponse)
			return
		}

		// Read request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err, "Failed to read request body")
		defer r.Body.Close()

		// Validate query if provided
		checkQuery(t, wantEsQuery, body)
		sendResponse(t, w, responseFile)
	}))
}

func checkQuery(t *testing.T, wantEsQuery string, body []byte) {
	if wantEsQuery != "" {
		var expected, actual map[string]any
		assert.NoError(t, json.Unmarshal([]byte(wantEsQuery), &expected))
		assert.NoError(t, json.Unmarshal(body, &actual))
		normalizeScripts(expected)
		normalizeScripts(actual)

		compareQueryStructure(t, expected, actual)
	}
}

func normalizeScripts(m any) {
	if m, ok := m.(map[string]any); ok {
		if script, ok := m["script"].(map[string]any); ok {
			if source, ok := script["source"].(string); ok {
				// Remove whitespace and newlines for comparison
				script["source"] = strings.Join(strings.Fields(source), " ")
			}
		}
		for _, v := range m {
			normalizeScripts(v)
		}
	}
}

func compareQueryStructure(t *testing.T, expected, actual map[string]any) {
	// Compare the bool query structure (without time ranges)
	if expectedQuery, ok := expected["query"].(map[string]any); ok {
		actualQuery := actual["query"].(map[string]any)
		compareBoolQuery(t, expectedQuery, actualQuery)
	}

	// Compare aggregations
	if expectedAggs, ok := expected["aggregations"].(map[string]any); ok {
		actualAggs := actual["aggregations"].(map[string]any)
		// For convenience, we remove date_histogram for easier comparison here because date_histogram includes time bounds which can vary by a few milliseconds
		removeHistogramBounds(expectedAggs)
		removeHistogramBounds(actualAggs)

		assert.Equal(t, expectedAggs, actualAggs, "Aggregations mismatch")
	}
}

// Simple helper to remove extended_bounds from any date_histogram
func removeHistogramBounds(aggs map[string]any) {
	for _, agg := range aggs {
		aggMap, ok := agg.(map[string]any)
		if !ok {
			continue
		}

		// Remove from date_histogram if present
		if histo, ok := aggMap["date_histogram"].(map[string]any); ok {
			delete(histo, "extended_bounds")
		}

		// Handle nested aggregations
		if nested, ok := aggMap["aggregations"].(map[string]any); ok {
			removeHistogramBounds(nested)
		}
	}
}

func compareBoolQuery(t *testing.T, expected, actual map[string]any) {
	expectedBool, eok := expected["bool"].(map[string]any)
	actualBool, aok := actual["bool"].(map[string]any)

	if !eok || !aok {
		return
	}

	// Compare filters (excluding time ranges)
	if expectedFilters, ok := expectedBool["filter"].([]any); ok {
		actualFilters := actualBool["filter"].([]any)
		compareFilters(t, expectedFilters, actualFilters)
	}
}

func compareFilters(t *testing.T, expected, actual []any) {
	// We'll compare the same number of filters, but skip time ranges
	assert.Len(t, actual, len(expected), "Different number of filters")

	for i := range expected {
		expectedFilter := expected[i].(map[string]any)
		actualFilter := actual[i].(map[string]any)

		// Skip range queries entirely
		if _, isRange := expectedFilter["range"]; isRange {
			continue
		}

		assert.Equal(t, expectedFilter, actualFilter, "Filter mismatch at index %d", i)
	}
}

func setupMetricsReaderFromServer(t *testing.T, mockServer *httptest.Server) (*MetricsReader, *tracetest.InMemoryExporter) {
	logger, _ := zap.NewDevelopment() // Use development logger for client-side logs
	tracer, exporter := tracerProvider(t)

	cfg := config.Configuration{
		Servers:  []string{mockServer.URL},
		LogLevel: "debug",
		Tags: config.TagsAsFields{
			Include:        "span.kind,error",
			DotReplacement: "@",
		},
	}

	client := clientProvider(t, &cfg, logger, esmetrics.NullFactory)
	reader := NewMetricsReader(client, cfg, logger, tracer)
	require.NotNil(t, reader)

	return reader, exporter
}

func buildTestBaseQueryParameters(tc metricsTestCase) metricstore.BaseQueryParameters {
	endTime := time.UnixMilli(1749894900000)
	lookback := 6 * time.Hour
	step := time.Minute
	ratePer := 10 * time.Minute

	return metricstore.BaseQueryParameters{
		ServiceNames:     tc.serviceNames,
		GroupByOperation: tc.groupByOp,
		EndTime:          &endTime,
		Lookback:         &lookback,
		Step:             &step,
		RatePer:          &ratePer,
		SpanKinds:        tc.spanKinds,
	}
}
