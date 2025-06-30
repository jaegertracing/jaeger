// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

func TestToMetricsFamily(t *testing.T) {
	tests := []struct {
		name     string
		params   MetricsQueryParams
		result   *elastic.SearchResult
		expected *metrics.MetricFamily
		err      string
	}{
		{
			name:   "successful conversion",
			params: mockMetricsQueryParams([]string{"service1"}, false),
			result: createTestSearchResult(false),
			expected: &metrics.MetricFamily{
				Name: "test_metric",
				Type: metrics.MetricType_GAUGE,
				Help: "test description",
				Metrics: []*metrics.Metric{
					{
						Labels: []*metrics.Label{
							{Name: "service_name", Value: "service1"},
						},
						MetricPoints: []*metrics.MetricPoint{
							createEpochGaugePoint(1.23),
						},
					},
				},
			},
		},
		{
			name: "missing aggregation",
			params: MetricsQueryParams{
				metricName: "test_metric",
			},
			result: &elastic.SearchResult{
				Aggregations: make(elastic.Aggregations),
			},
			err: "results_buckets aggregation not found",
		},
	}

	translator := Translator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translator.ToMetricsFamily(tt.params, tt.result)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestToDomainMetrics(t *testing.T) {
	tests := []struct {
		name     string
		params   MetricsQueryParams
		result   *elastic.SearchResult
		expected []*metrics.Metric
		err      string
	}{
		{
			name:   "simple metrics",
			params: mockMetricsQueryParams([]string{"service1"}, false),
			result: createTestSearchResult(false),
			expected: []*metrics.Metric{
				{
					Labels: []*metrics.Label{
						{Name: "service_name", Value: "service1"},
					},
					MetricPoints: []*metrics.MetricPoint{
						createEpochGaugePoint(1.23),
					},
				},
			},
		},
		{
			name:   "grouped by operation",
			params: mockMetricsQueryParams([]string{"service1"}, true),
			result: createTestSearchResult(true),
			expected: []*metrics.Metric{
				{
					Labels: []*metrics.Label{
						{Name: "service_name", Value: "service1"},
						{Name: "operation", Value: "op1"},
					},
					MetricPoints: []*metrics.MetricPoint{
						createEpochGaugePoint(1.23),
					},
				},
			},
		},
	}

	translator := Translator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translator.toDomainMetrics(tt.params, tt.result)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestToDomainMetrics_ErrorCases(t *testing.T) {
	tests := []struct {
		name   string
		params MetricsQueryParams
		result *elastic.SearchResult
		errMsg string
	}{
		{
			name:   "missing terms aggregation when group by operation",
			params: mockMetricsQueryParams([]string{"service1"}, true),
			result: &elastic.SearchResult{
				Aggregations: make(elastic.Aggregations), // Empty aggregations
			},
			errMsg: "results_buckets aggregation not found",
		},
		{
			name:   "bucket key not string",
			params: mockMetricsQueryParams([]string{"service1"}, true),
			result: createTestSearchResultWithNonStringKey(),
			errMsg: "bucket key is not a string",
		},
		{
			name:   "missing date histogram in operation bucket",
			params: mockMetricsQueryParams([]string{"service1"}, true),
			result: createTestSearchResultMissingDateHistogram(),
			errMsg: "date_histogram aggregation not found in bucket",
		},
	}

	translator := Translator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translator.toDomainMetrics(tt.params, tt.result)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func createEpochGaugePoint(value float64) *metrics.MetricPoint {
	return &metrics.MetricPoint{
		Value: &metrics.MetricPoint_GaugeValue{
			GaugeValue: &metrics.GaugeValue{
				Value: &metrics.GaugeValue_DoubleValue{DoubleValue: value},
			},
		},
		Timestamp: mustTimestampProto(time.Unix(0, 0)),
	}
}

// mockMetricsQueryParams creates a MetricsQueryParams struct for testing.
func mockMetricsQueryParams(serviceNames []string, groupByOp bool) MetricsQueryParams {
	return MetricsQueryParams{
		metricName: "test_metric",
		metricDesc: "test description",
		BaseQueryParameters: metricstore.BaseQueryParameters{
			ServiceNames:     serviceNames,
			GroupByOperation: groupByOp,
		},
		bucketsToPointsFunc: func(buckets []*elastic.AggregationBucketHistogramItem) []*Pair {
			return []*Pair{newPair(0, 1.23)}
		},
		processMetricsFunc: func(pair []*Pair, _ metricstore.BaseQueryParameters) []*Pair {
			return pair
		},
	}
}

// createTestSearchResultWithNonStringKey creates an Elasticsearch SearchResult
// where the bucket key for operation is an integer, causing a type error.
func createTestSearchResultWithNonStringKey() *elastic.SearchResult {
	rawAggregation := json.RawMessage(`{
		"buckets": [{
			"key": 12345,
			"doc_count": 10,
			"date_histogram": {
				"buckets": [{
					"key": 123456,
					"doc_count": 5,
					"results": {"value": 1.23}
				}]
			}
		}]
	}`)

	aggs := make(elastic.Aggregations)
	aggs[aggName] = rawAggregation

	return &elastic.SearchResult{
		Aggregations: aggs,
	}
}

// createTestSearchResultMissingDateHistogram creates an Elasticsearch SearchResult
// where an operation bucket is missing the expected date_histogram aggregation.
func createTestSearchResultMissingDateHistogram() *elastic.SearchResult {
	rawAggregation := json.RawMessage(`{
		"buckets": [{
			"key": "op1",
			"doc_count": 10
		}]
	}`)

	aggs := make(elastic.Aggregations)
	aggs[aggName] = rawAggregation

	return &elastic.SearchResult{
		Aggregations: aggs,
	}
}

// createTestSearchResult creates a well-formed Elasticsearch SearchResult
// for testing successful conversions, with or without operation grouping.
func createTestSearchResult(groupByOperation bool) *elastic.SearchResult {
	var rawAggregation json.RawMessage

	if groupByOperation {
		rawAggregation = json.RawMessage(`{
			"buckets": [{
				"key": "op1",
				"doc_count": 10,
				"date_histogram": {
					"buckets": [{
						"key_as_string": "123456",
						"key": 123456,
						"doc_count": 5,
						"cumulative_requests": {
							"value": 1.23
						}
					}]
				}
			}]
		}`)
	} else {
		rawAggregation = json.RawMessage(`{
			"buckets": [{
				"key_as_string": "123456",
				"key": 123456,
				"doc_count": 5,
				"cumulative_requests": {
					"value": 1.23
				}
			}]
		}`)
	}

	aggs := make(elastic.Aggregations)
	aggs[aggName] = rawAggregation

	return &elastic.SearchResult{
		Aggregations: aggs,
	}
}

func mustTimestampProto(t time.Time) *types.Timestamp {
	ts, err := types.TimestampProto(t)
	if err != nil {
		panic(err)
	}
	return ts
}
