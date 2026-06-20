// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const summaryAggregationJSON = `{
  "buckets": [
    {
      "key": "00000000000000000000000000000001",
      "doc_count": 3,
      "min_start": {"value": 1000000},
      "max_start": {"value": 1500000},
      "max_end": {"value": 2000000},
      "error_count": {"doc_count": 1},
      "services": {"buckets": [
        {"key": "svcB", "doc_count": 1, "service_errors": {"doc_count": 0}},
        {"key": "svcA", "doc_count": 2, "service_errors": {"doc_count": 1}}
      ]},
      "root": {"doc_count": 1, "root_hit": {"hits": {"hits": [
        {"_source": {"operationName": "root-op", "process": {"serviceName": "svcA"}}}
      ]}}}
    }
  ]
}`

func mockSummarySearchService(r *spanReaderTest) *mock.Call {
	searchService := &mocks.SearchService{}
	searchService.On("Query", mock.Anything).Return(searchService)
	searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
	searchService.On("Size", mock.AnythingOfType("int")).Return(searchService)
	searchService.On("Aggregation", mock.AnythingOfType("string"), mock.Anything).Return(searchService)
	r.client.On("Search", mock.AnythingOfType("[]string")).Return(searchService)
	return searchService.On("Do", mock.Anything)
}

func validSummaryQuery() dbmodel.TraceQueryParameters {
	return dbmodel.TraceQueryParameters{
		ServiceName:  serviceName,
		StartTimeMin: time.Now().Add(-time.Hour),
		StartTimeMax: time.Now(),
		SearchDepth:  10,
	}
}

func TestSpanReader_FindTraceSummaries(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		aggs := map[string]json.RawMessage{traceSummariesAggregation: []byte(summaryAggregationJSON)}
		mockSummarySearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(aggs)}, nil)

		summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.NoError(t, err)
		require.Len(t, summaries, 1)

		s := summaries[0]
		assert.Equal(t, dbmodel.TraceID("00000000000000000000000000000001"), s.TraceID)
		assert.Equal(t, 3, s.SpanCount)
		assert.Equal(t, 1, s.ErrorSpanCount)
		assert.Equal(t, uint64(1000000), s.MinStartTime)
		assert.Equal(t, uint64(2000000), s.MaxEndTime)
		assert.Equal(t, "svcA", s.RootServiceName)
		assert.Equal(t, "root-op", s.RootOperationName)

		require.Len(t, s.Services, 2)
		// Sorted by service name.
		assert.Equal(t, "svcA", s.Services[0].ServiceName)
		assert.Equal(t, 2, s.Services[0].SpanCount)
		assert.Equal(t, 1, s.Services[0].ErrorSpanCount)
		assert.Equal(t, "svcB", s.Services[1].ServiceName)
		assert.Equal(t, 1, s.Services[1].SpanCount)
		assert.Equal(t, 0, s.Services[1].ErrorSpanCount)
	})
}

func TestSpanReader_FindTraceSummaries_InvalidQuery(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// Missing start/end time fails validation before any search is issued.
		_, err := r.reader.FindTraceSummaries(context.Background(), dbmodel.TraceQueryParameters{ServiceName: serviceName})
		require.Error(t, err)
	})
}

func TestSpanReader_FindTraceSummaries_SearchError(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).Return(nil, errors.New("search failed"))
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.Error(t, err)
	})
}

func TestSpanReader_FindTraceSummaries_NoAggregations(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).Return(&elastic.SearchResult{Aggregations: nil}, nil)
		summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.NoError(t, err)
		assert.Empty(t, summaries)
	})
}

func TestSpanReader_FindTraceSummaries_BadRootSource(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// A malformed root-span _source must surface as an error, not be silently
		// turned into an empty root service/operation.
		badRoot := `{
  "buckets": [
    {
      "key": "00000000000000000000000000000001",
      "doc_count": 1,
      "min_start": {"value": 1000000},
      "max_end": {"value": 2000000},
      "error_count": {"doc_count": 0},
      "services": {"buckets": []},
      "root": {"doc_count": 1, "root_hit": {"hits": {"hits": [
        {"_source": "not-an-object"}
      ]}}}
    }
  ]
}`
		aggs := map[string]json.RawMessage{traceSummariesAggregation: []byte(badRoot)}
		mockSummarySearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(aggs)}, nil)
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.Error(t, err)
	})
}

func TestSpanReader_FindTraceSummaries_MissingBucketAggregation(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		aggs := map[string]json.RawMessage{"other": []byte(`{"buckets": []}`)}
		mockSummarySearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(aggs)}, nil)
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.ErrorIs(t, err, ErrUnableToFindTraceIDAggregation)
	})
}
