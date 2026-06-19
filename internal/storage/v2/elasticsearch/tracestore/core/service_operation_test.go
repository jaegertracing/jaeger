// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func TestWriteService(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		// Hash of "service||operation" (service=service, spanKind="", operation=operation)
		serviceHash := "d17b3510df3ba545"

		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		indexService.On("Add")

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID:       dbmodel.TraceID("1"),
			SpanID:        dbmodel.SpanID("0"),
			OperationName: "operation",
			Process: dbmodel.Process{
				ServiceName: "service",
			},
		}

		w.writer.writeService(indexName, jsonSpan)

		indexService.AssertNumberOfCalls(t, "Add", 1)
		assert.Empty(t, w.logBuffer.String())

		// test that cache works, will call the index service only once.
		w.writer.writeService(indexName, jsonSpan)
		indexService.AssertNumberOfCalls(t, "Add", 1)
	})
}

func TestWriteServiceWithSpanKind(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		// Hash of "service|client|operation"
		serviceHash := "cbeb6711d7d10522"

		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		indexService.On("Add")

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID:       dbmodel.TraceID("1"),
			SpanID:        dbmodel.SpanID("0"),
			OperationName: "operation",
			Tags: []dbmodel.KeyValue{
				{Key: spanKindTagKey, Value: "client"},
			},
			Process: dbmodel.Process{
				ServiceName: "service",
			},
		}

		w.writer.writeService(indexName, jsonSpan)

		indexService.AssertNumberOfCalls(t, "Add", 1)
		assert.Empty(t, w.logBuffer.String())
	})
}

func TestWriteServiceError(*testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		serviceHash := "d17b3510df3ba545"

		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		indexService.On("Add")

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID:       dbmodel.TraceID("1"),
			SpanID:        dbmodel.SpanID("0"),
			OperationName: "operation",
			Process: dbmodel.Process{
				ServiceName: "service",
			},
		}

		w.writer.writeService(indexName, jsonSpan)
	})
}

func TestSpanReader_GetServices(t *testing.T) {
	testGet(servicesAggregation, t)
}

func TestSpanReader_GetOperations(t *testing.T) {
	testGet(operationsAggregation, t)
}

func TestSpanReader_GetOperationsWithSpanKind(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		searchService := &mocks.SearchService{}

		expectedQuery := elastic.NewBoolQuery().Must(
			elastic.NewTermQuery(serviceName, "myService"),
			elastic.NewTermQuery(spanKindField, "server"),
		)

		searchService.On("Query", mock.MatchedBy(func(query elastic.Query) bool {
			actualSource, err := query.Source()
			require.NoError(t, err)
			expectedSource, err := expectedQuery.Source()
			require.NoError(t, err)
			return reflect.DeepEqual(actualSource, expectedSource)
		})).Return(searchService)
		searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
		searchService.On("Size", 0).Return(searchService)
		searchService.On("Aggregation", stringMatcher(operationsAggregation), mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("[]string")).Return(searchService)

		// Aggregation result that includes a spanKind sub-aggregation.
		rawMessage, err := json.Marshal(map[string]any{
			"buckets": []map[string]any{
				{
					"key":       "myOperation",
					"doc_count": 3,
					spanKindAggregation: map[string]any{
						"buckets": []map[string]any{
							{"key": "server", "doc_count": 3},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		aggs := elastic.Aggregations(map[string]json.RawMessage{
			operationsAggregation: rawMessage,
		})
		searchService.On("Do", mock.Anything).Return(&elastic.SearchResult{Aggregations: aggs}, nil)

		ops, err := r.reader.GetOperations(
			context.Background(),
			dbmodel.OperationQueryParameters{ServiceName: "myService", SpanKind: "server"},
		)
		require.NoError(t, err)
		assert.Equal(t, []dbmodel.Operation{
			{Name: "myOperation", SpanKind: "server"},
		}, ops)
	})
}

func TestSpanReader_GetServicesEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)
		services, err := r.reader.GetServices(context.Background())
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestSpanReader_GetOperationsEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)
		services, err := r.reader.GetOperations(
			context.Background(),
			dbmodel.OperationQueryParameters{ServiceName: "foo"},
		)
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestGetSpanKindFromSpan(t *testing.T) {
	tests := []struct {
		name     string
		span     *dbmodel.Span
		expected string
	}{
		{
			name: "span kind from Tags slice",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{
					{Key: spanKindTagKey, Value: "server"},
				},
			},
			expected: "server",
		},
		{
			name: "span kind from flat Tag map",
			span: &dbmodel.Span{
				Tag: map[string]any{spanKindTagKey: "client"},
			},
			expected: "client",
		},
		{
			name:     "no span kind returns empty string",
			span:     &dbmodel.Span{},
			expected: "",
		},
		{
			name: "Tags slice takes priority over Tag map",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{
					{Key: spanKindTagKey, Value: "producer"},
				},
				Tag: map[string]any{spanKindTagKey: "consumer"},
			},
			expected: "producer",
		},
		{
			name: "non-string tag value is skipped",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{
					{Key: spanKindTagKey, Value: 42},
				},
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getSpanKindFromSpan(tc.span))
		})
	}
}

func TestOperationsBucketToOperations_InvalidOperationNameKey(t *testing.T) {
	_, err := operationsBucketToOperations([]*elastic.AggregationBucketKeyItem{
		{Key: 123},
	})

	require.EqualError(t, err, "could not convert operation name bucket key to string")
}

func TestOperationsBucketToOperations_InvalidSpanKindKey(t *testing.T) {
	rawMessage, err := json.Marshal(map[string]any{
		"buckets": []map[string]any{
			{
				"key":       "myOperation",
				"doc_count": 1,
				spanKindAggregation: map[string]any{
					"buckets": []map[string]any{
						{"key": 123, "doc_count": 1},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	var bucket elastic.AggregationBucketKeyItems
	require.NoError(t, json.Unmarshal(rawMessage, &bucket))

	_, err = operationsBucketToOperations(bucket.Buckets)
	require.EqualError(t, err, "could not convert span kind bucket key to string")
}
