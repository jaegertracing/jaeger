// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

func TestNewTranslator(t *testing.T) {
	translator := NewTranslator()
	assert.NotNil(t, translator)
}

func TestToMetricsFamily(t *testing.T) {
	tests := []struct {
		name     string
		params   MetricsQueryParams
		result   *elastic.SearchResult
		expected *metrics.MetricFamily
		err      string
	}{
		{
			name: "successful conversion",
			params: MetricsQueryParams{
				metricName: "test_metric",
				metricDesc: "test description",
				BaseQueryParameters: metricstore.BaseQueryParameters{
					ServiceNames: []string{"service1"},
				},
			},
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
							{
								Value: &metrics.MetricPoint_GaugeValue{
									GaugeValue: &metrics.GaugeValue{
										Value: &metrics.GaugeValue_DoubleValue{DoubleValue: 1.23},
									},
								},
								Timestamp: mustTimestampProto(time.Unix(0, 123456*int64(time.Millisecond))),
							},
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

	translator := NewTranslator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translator.ToMetricsFamily(tt.params, tt.result)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
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
			name: "simple metrics",
			params: MetricsQueryParams{
				BaseQueryParameters: metricstore.BaseQueryParameters{
					ServiceNames: []string{"service1"},
				},
			},
			result: createTestSearchResult(false),
			expected: []*metrics.Metric{
				{
					Labels: []*metrics.Label{
						{Name: "service_name", Value: "service1"},
					},
					MetricPoints: []*metrics.MetricPoint{
						{
							Value: &metrics.MetricPoint_GaugeValue{
								GaugeValue: &metrics.GaugeValue{
									Value: &metrics.GaugeValue_DoubleValue{DoubleValue: 1.23},
								},
							},
							Timestamp: mustTimestampProto(time.Unix(0, 123456*int64(time.Millisecond))),
						},
					},
				},
			},
		},
		{
			name: "grouped by operation",
			params: MetricsQueryParams{
				BaseQueryParameters: metricstore.BaseQueryParameters{
					ServiceNames:     []string{"service1"},
					GroupByOperation: true,
				},
			},
			result: createTestSearchResult(true),
			expected: []*metrics.Metric{
				{
					Labels: []*metrics.Label{
						{Name: "service_name", Value: "service1"},
						{Name: "operation", Value: "op1"},
					},
					MetricPoints: []*metrics.MetricPoint{
						{
							Value: &metrics.MetricPoint_GaugeValue{
								GaugeValue: &metrics.GaugeValue{
									Value: &metrics.GaugeValue_DoubleValue{DoubleValue: 1.23},
								},
							},
							Timestamp: mustTimestampProto(time.Unix(0, 123456*int64(time.Millisecond))),
						},
					},
				},
			},
		},
	}

	translator := NewTranslator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translator.toDomainMetrics(tt.params, tt.result)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// Helper functions to create properly typed test data

func createTestSearchResult(groupByOperation bool) *elastic.SearchResult {
	// Create raw aggregation JSON that matches what Elasticsearch would return
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
						"results": {
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
				"results": {
					"value": 1.23
				}
			}]
		}`)
	}

	aggs := make(elastic.Aggregations)
	aggs["results_buckets"] = &rawAggregation

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
