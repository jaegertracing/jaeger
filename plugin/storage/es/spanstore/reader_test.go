// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const defaultMaxDocCount = 10_000

var exampleESSpan = []byte(
	`{
	   "traceID": "1",
	   "parentSpanID": "2",
	   "spanID": "3",
	   "flags": 0,
	   "operationName": "op",
	   "references": [],
	   "startTime": 812965625,
	   "duration": 3290114992,
	   "tags": [
	      {
		 "key": "tag",
		 "value": "1965806585",
		 "type": "int64"
	      }
	   ],
	   "logs": [
	      {
		 "timestamp": 812966073,
		 "fields": [
		    {
		       "key": "logtag",
		       "value": "helloworld",
		       "type": "string"
		    }
		 ]
	      }
	   ],
	   "process": {
	      "serviceName": "serv",
	      "tags": [
		 {
		    "key": "processtag",
		    "value": "false",
		    "type": "bool"
		 }
	      ]
	   }
	}`)

type spanReaderTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	reader    *SpanReader
}

func withSpanReader(fn func(r *spanReaderTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &spanReaderTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		reader: NewSpanReader(SpanReaderParams{
			Client:            client,
			Logger:            zap.NewNop(),
			MaxSpanAge:        0,
			IndexPrefix:       "",
			TagDotReplacement: "@",
			MaxDocCount:       defaultMaxDocCount,
		}),
	}
	fn(r)
}

func withArchiveSpanReader(readAlias bool, fn func(r *spanReaderTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &spanReaderTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		reader: NewSpanReader(SpanReaderParams{
			Client:              client,
			Logger:              zap.NewNop(),
			MaxSpanAge:          0,
			IndexPrefix:         "",
			TagDotReplacement:   "@",
			Archive:             true,
			UseReadWriteAliases: readAlias,
		}),
	}
	fn(r)
}

var _ spanstore.Reader = &SpanReader{} // check API conformance

func TestNewSpanReader(t *testing.T) {
	client := &mocks.Client{}
	reader := NewSpanReader(SpanReaderParams{
		Client:         client,
		Logger:         zap.NewNop(),
		MaxSpanAge:     0,
		MetricsFactory: metrics.NullFactory,
		IndexPrefix:    ""})
	assert.NotNil(t, reader)
}

func TestSpanReaderIndices(t *testing.T) {
	client := &mocks.Client{}
	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	date := time.Date(2019, 10, 10, 5, 0, 0, 0, time.UTC)
	dateFormat := date.UTC().Format("2006-01-02")
	testCases := []struct {
		index  string
		params SpanReaderParams
	}{
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "", Archive: false},
			index: spanIndex + dateFormat},
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "", UseReadWriteAliases: true},
			index: spanIndex + "read"},
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "foo:", Archive: false},
			index: "foo:" + indexPrefixSeparator + spanIndex + dateFormat},
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "foo:", UseReadWriteAliases: true},
			index: "foo:-" + spanIndex + "read"},
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "", Archive: true},
			index: spanIndex + archiveIndexSuffix},
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "foo:", Archive: true},
			index: "foo:" + indexPrefixSeparator + spanIndex + archiveIndexSuffix},
		{params: SpanReaderParams{Client: client, Logger: logger, MetricsFactory: metricsFactory,
			IndexPrefix: "foo:", Archive: true, UseReadWriteAliases: true},
			index: "foo:" + indexPrefixSeparator + spanIndex + archiveReadIndexSuffix},
	}
	for _, testCase := range testCases {
		r := NewSpanReader(testCase.params)
		actual := r.timeRangeIndices(r.spanIndexPrefix, "2006-01-02", date, date)
		assert.Equal(t, []string{testCase.index}, actual)
	}
}

func TestSpanReader_GetTrace(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		hits := make([]*elastic.SearchHit, 1)
		hits[0] = &elastic.SearchHit{
			Source: (*json.RawMessage)(&exampleESSpan),
		}
		searchHits := &elastic.SearchHits{Hits: hits}

		mockSearchService(r).Return(&elastic.SearchResult{Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
				},
			}, nil)

		trace, err := r.reader.GetTrace(context.Background(), model.NewTraceID(0, 1))
		require.NoError(t, err)
		require.NotNil(t, trace)

		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		require.Len(t, trace.Spans, 1)
		assert.EqualValues(t, trace.Spans[0], expectedSpans[0])
	})
}

func TestSpanReader_multiRead_followUp_query(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		date := time.Date(2019, 10, 10, 5, 0, 0, 0, time.UTC)
		spanID1 := dbmodel.Span{SpanID: "0", TraceID: "1", StartTime: model.TimeAsEpochMicroseconds(date)}
		spanBytesID1, err := json.Marshal(spanID1)
		require.NoError(t, err)
		spanID2 := dbmodel.Span{SpanID: "0", TraceID: "2", StartTime: model.TimeAsEpochMicroseconds(date)}
		spanBytesID2, err := json.Marshal(spanID2)
		require.NoError(t, err)

		id1Query := elastic.NewBoolQuery().Should(
			elastic.NewTermQuery(traceIDField, model.TraceID{High: 0, Low: 1}.String()).Boost(2),
			elastic.NewTermQuery(traceIDField, fmt.Sprintf("%x", 1)))
		id1Search := elastic.NewSearchRequest().
			IgnoreUnavailable(true).
			Source(r.reader.sourceFn(id1Query, model.TimeAsEpochMicroseconds(date.Add(-time.Hour))))
		id2Query := elastic.NewBoolQuery().Should(
			elastic.NewTermQuery(traceIDField, model.TraceID{High: 0, Low: 2}.String()).Boost(2),
			elastic.NewTermQuery(traceIDField, fmt.Sprintf("%x", 2)))
		id2Search := elastic.NewSearchRequest().
			IgnoreUnavailable(true).
			Source(r.reader.sourceFn(id2Query, model.TimeAsEpochMicroseconds(date.Add(-time.Hour))))
		id1SearchSpanTime := elastic.NewSearchRequest().
			IgnoreUnavailable(true).
			Source(r.reader.sourceFn(id1Query, spanID1.StartTime))

		multiSearchService := &mocks.MultiSearchService{}
		firstMultiSearch := &mocks.MultiSearchService{}
		secondMultiSearch := &mocks.MultiSearchService{}
		multiSearchService.On("Add", id1Search, id2Search).Return(firstMultiSearch)
		multiSearchService.On("Add", id1SearchSpanTime).Return(secondMultiSearch)

		firstMultiSearch.On("Index", mock.AnythingOfType("string")).Return(firstMultiSearch)
		secondMultiSearch.On("Index", mock.AnythingOfType("string")).Return(secondMultiSearch)
		r.client.On("MultiSearch").Return(multiSearchService)

		fistMultiSearchMock := firstMultiSearch.On("Do", mock.AnythingOfType("*context.emptyCtx"))
		secondMultiSearchMock := secondMultiSearch.On("Do", mock.AnythingOfType("*context.emptyCtx"))

		// set TotalHits to two to trigger the follow up query
		// the client will return only one span therefore the implementation
		// triggers follow up query for the same traceID with the timestamp of the last span
		searchHitsID1 := &elastic.SearchHits{Hits: []*elastic.SearchHit{
			{Source: (*json.RawMessage)(&spanBytesID1)},
		}, TotalHits: 2}
		fistMultiSearchMock.
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHitsID1},
				},
			}, nil)

		searchHitsID2 := &elastic.SearchHits{Hits: []*elastic.SearchHit{
			{Source: (*json.RawMessage)(&spanBytesID2)},
		}, TotalHits: 1}
		secondMultiSearchMock.
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHitsID2},
				},
			}, nil)

		traces, err := r.reader.multiRead(context.Background(), []model.TraceID{{High: 0, Low: 1}, {High: 0, Low: 2}}, date, date)
		require.NoError(t, err)
		require.NotNil(t, traces)
		require.Len(t, traces, 2)

		toDomain := dbmodel.NewToDomain("-")
		sModel1, err := toDomain.SpanToDomain(&spanID1)
		require.NoError(t, err)
		sModel2, err := toDomain.SpanToDomain(&spanID2)
		require.NoError(t, err)

		for _, s := range []*model.Span{sModel1, sModel2} {
			found := reflect.DeepEqual(traces[0].Spans[0], s) || reflect.DeepEqual(traces[1].Spans[0], s)
			assert.True(t, found, "span was expected to be within one of the traces but was not: %v", s)
		}
	})
}

func TestSpanReader_SearchAfter(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		var hits []*elastic.SearchHit

		for i := 0; i < 10000; i++ {
			hit := &elastic.SearchHit{Source: (*json.RawMessage)(&exampleESSpan)}
			hits = append(hits, hit)
		}

		searchHits := &elastic.SearchHits{Hits: hits, TotalHits: int64(10040)}

		mockSearchService(r).Return(&elastic.SearchResult{Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
				},
			}, nil).Times(2)

		trace, err := r.reader.GetTrace(context.Background(), model.NewTraceID(0, 1))
		require.NoError(t, err)
		require.NotNil(t, trace)

		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		assert.EqualValues(t, trace.Spans[0], expectedSpans[0])
	})
}

func TestSpanReader_GetTraceQueryError(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(nil, errors.New("query error occurred"))
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)
		trace, err := r.reader.GetTrace(context.Background(), model.NewTraceID(0, 1))
		require.EqualError(t, err, "trace not found")
		require.Nil(t, trace)
	})
}

func TestSpanReader_GetTraceNilHits(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		var hits []*elastic.SearchHit
		searchHits := &elastic.SearchHits{Hits: hits}

		mockSearchService(r).Return(&elastic.SearchResult{Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: nil},
				},
			}, nil)

		trace, err := r.reader.GetTrace(context.Background(), model.NewTraceID(0, 1))
		require.EqualError(t, err, "trace not found")
		require.Nil(t, trace)
	})
}

func TestSpanReader_GetTraceInvalidSpanError(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		data := []byte(`{"TraceID": "123"asdf fadsg}`)
		hits := make([]*elastic.SearchHit, 1)
		hits[0] = &elastic.SearchHit{
			Source: (*json.RawMessage)(&data),
		}
		searchHits := &elastic.SearchHits{Hits: hits}

		mockSearchService(r).Return(&elastic.SearchResult{Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
				},
			}, nil)

		trace, err := r.reader.GetTrace(context.Background(), model.NewTraceID(0, 1))
		require.Error(t, err, "invalid span")
		require.Nil(t, trace)
	})
}

func TestSpanReader_GetTraceSpanConversionError(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		badSpan := []byte(`{"TraceID": "123"}`)

		hits := make([]*elastic.SearchHit, 1)
		hits[0] = &elastic.SearchHit{
			Source: (*json.RawMessage)(&badSpan),
		}
		searchHits := &elastic.SearchHits{Hits: hits}

		mockSearchService(r).Return(&elastic.SearchResult{Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
				},
			}, nil)

		trace, err := r.reader.GetTrace(context.Background(), model.NewTraceID(0, 1))
		require.Error(t, err, "span conversion error, because lacks elements")
		require.Nil(t, trace)
	})
}

func TestSpanReader_esJSONtoJSONSpanModel(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		jsonPayload := (*json.RawMessage)(&exampleESSpan)

		esSpanRaw := &elastic.SearchHit{
			Source: jsonPayload,
		}

		span, err := r.reader.unmarshalJSONSpan(esSpanRaw)
		require.NoError(t, err)

		var expectedSpan dbmodel.Span
		require.NoError(t, json.Unmarshal(exampleESSpan, &expectedSpan))
		assert.EqualValues(t, &expectedSpan, span)
	})
}

func TestSpanReader_esJSONtoJSONSpanModelError(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		data := []byte(`{"TraceID": "123"asdf fadsg}`)
		jsonPayload := (*json.RawMessage)(&data)

		esSpanRaw := &elastic.SearchHit{
			Source: jsonPayload,
		}

		span, err := r.reader.unmarshalJSONSpan(esSpanRaw)
		require.Error(t, err)
		assert.Nil(t, span)
	})
}

func TestSpanReaderFindIndices(t *testing.T) {
	today := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)
	dateLayout := "2006-01-02"

	testCases := []struct {
		startTime time.Time
		endTime   time.Time
		expected  []string
	}{
		{
			startTime: today.Add(-time.Millisecond),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndex, dateLayout, today),
			},
		},
		{
			startTime: today.Add(-13 * time.Hour),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndex, dateLayout, today),
				indexWithDate(spanIndex, dateLayout, yesterday),
			},
		},
		{
			startTime: today.Add(-48 * time.Hour),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndex, dateLayout, today),
				indexWithDate(spanIndex, dateLayout, yesterday),
				indexWithDate(spanIndex, dateLayout, twoDaysAgo),
			},
		},
	}
	withSpanReader(func(r *spanReaderTest) {
		for _, testCase := range testCases {
			actual := r.reader.timeRangeIndices(spanIndex, dateLayout, testCase.startTime, testCase.endTime)
			assert.EqualValues(t, testCase.expected, actual)
		}
	})
}

func TestSpanReader_indexWithDate(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		actual := indexWithDate(spanIndex, "2006-01-02", time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC))
		assert.Equal(t, "jaeger-span-1995-04-21", actual)
	})
}

func testGet(typ string, t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": [{"key": "123","doc_count": 16}]}`)
	goodAggregations[typ] = (*json.RawMessage)(&rawMessage)

	badAggregations := make(map[string]*json.RawMessage)
	badRawMessage := []byte(`{"buckets": [{bad json]}asdf`)
	badAggregations[typ] = (*json.RawMessage)(&badRawMessage)

	testCases := []struct {
		caption        string
		searchResult   *elastic.SearchResult
		searchError    error
		expectedError  func() string
		expectedOutput map[string]interface{}
	}{
		{
			caption:      typ + " full behavior",
			searchResult: &elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations)},
			expectedOutput: map[string]interface{}{
				operationsAggregation: []spanstore.Operation{{Name: "123"}},
				"default":             []string{"123"},
			},
			expectedError: func() string {
				return ""
			},
		},
		{
			caption:     typ + " search error",
			searchError: errors.New("Search failure"),
			expectedError: func() string {
				if typ == operationsAggregation {
					return "search operations failed: Search failure"
				}
				return "search services failed: Search failure"
			},
		},
		{
			caption:      typ + " search error",
			searchResult: &elastic.SearchResult{Aggregations: elastic.Aggregations(badAggregations)},
			expectedError: func() string {
				return "could not find aggregation of " + typ
			},
		},
	}

	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanReader(func(r *spanReaderTest) {
				mockSearchService(r).Return(testCase.searchResult, testCase.searchError)
				actual, err := returnSearchFunc(typ, r)
				if testCase.expectedError() != "" {
					require.EqualError(t, err, testCase.expectedError())
					assert.Nil(t, actual)
				} else if expectedOutput, ok := testCase.expectedOutput[typ]; ok {
					assert.EqualValues(t, expectedOutput, actual)
				} else {
					assert.EqualValues(t, testCase.expectedOutput["default"], actual)
				}
			})
		})
	}
}

func returnSearchFunc(typ string, r *spanReaderTest) (interface{}, error) {
	if typ == servicesAggregation {
		return r.reader.GetServices(context.Background())
	} else if typ == operationsAggregation {
		return r.reader.GetOperations(
			context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "someService"},
		)
	} else if typ == traceIDAggregation {
		return r.reader.findTraceIDs(context.Background(), &spanstore.TraceQueryParameters{})
	}
	return nil, errors.New("Specify services, operations, traceIDs only")
}
func TestSpanReader_bucketToStringArray(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		buckets := make([]*elastic.AggregationBucketKeyItem, 3)
		buckets[0] = &elastic.AggregationBucketKeyItem{Key: "hello"}
		buckets[1] = &elastic.AggregationBucketKeyItem{Key: "world"}
		buckets[2] = &elastic.AggregationBucketKeyItem{Key: "2"}

		actual, err := bucketToStringArray(buckets)
		require.NoError(t, err)

		assert.EqualValues(t, []string{"hello", "world", "2"}, actual)
	})
}

func TestSpanReader_bucketToStringArrayError(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		buckets := make([]*elastic.AggregationBucketKeyItem, 3)
		buckets[0] = &elastic.AggregationBucketKeyItem{Key: "hello"}
		buckets[1] = &elastic.AggregationBucketKeyItem{Key: "world"}
		buckets[2] = &elastic.AggregationBucketKeyItem{Key: 2}

		_, err := bucketToStringArray(buckets)
		assert.EqualError(t, err, "non-string key found in aggregation")
	})
}

func TestSpanReader_FindTraces(t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": [{"key": "1","doc_count": 16},{"key": "2","doc_count": 16},{"key": "3","doc_count": 16}]}`)
	goodAggregations[traceIDAggregation] = (*json.RawMessage)(&rawMessage)

	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&exampleESSpan),
	}
	searchHits := &elastic.SearchHits{Hits: hits}

	withSpanReader(func(r *spanReaderTest) {
		// find trace IDs
		mockSearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations), Hits: searchHits}, nil)
		// bulk read traces
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
					{Hits: searchHits},
				},
			}, nil)

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    1,
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NoError(t, err)
		assert.Len(t, traces, 1)

		trace := traces[0]
		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		require.Len(t, trace.Spans, 2)
		assert.EqualValues(t, trace.Spans[0], expectedSpans[0])
	})
}

func TestSpanReader_FindTracesInvalidQuery(t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": [{"key": "1","doc_count": 16},{"key": "2","doc_count": 16},{"key": "3","doc_count": 16}]}`)
	goodAggregations[traceIDAggregation] = (*json.RawMessage)(&rawMessage)

	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&exampleESSpan),
	}
	searchHits := &elastic.SearchHits{Hits: hits}

	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations), Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
					{Hits: searchHits},
				},
			}, nil)

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: "",
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.Error(t, err)
		assert.Nil(t, traces)
	})
}

func TestSpanReader_FindTracesAggregationFailure(t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)

	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&exampleESSpan),
	}
	searchHits := &elastic.SearchHits{Hits: hits}

	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations), Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.Error(t, err)
		assert.Nil(t, traces)
	})
}

func TestSpanReader_FindTracesNoTraceIDs(t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": []}`)
	goodAggregations[traceIDAggregation] = (*json.RawMessage)(&rawMessage)

	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&exampleESSpan),
	}
	searchHits := &elastic.SearchHits{Hits: hits}

	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations), Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NoError(t, err)
		assert.Len(t, traces, 0)
	})
}

func TestSpanReader_FindTracesReadTraceFailure(t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": [{"key": "1","doc_count": 16},{"key": "2","doc_count": 16}]}`)
	goodAggregations[traceIDAggregation] = (*json.RawMessage)(&rawMessage)

	badSpan := []byte(`{"TraceID": "123"asjlgajdfhilqghi[adfvca} bad json`)
	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&badSpan),
	}
	searchHits := &elastic.SearchHits{Hits: hits}

	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations), Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(nil, errors.New("read error"))

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.EqualError(t, err, "read error")
		assert.Len(t, traces, 0)
	})
}

func TestSpanReader_FindTracesSpanCollectionFailure(t *testing.T) {
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": [{"key": "1","doc_count": 16},{"key": "2","doc_count": 16}]}`)
	goodAggregations[traceIDAggregation] = (*json.RawMessage)(&rawMessage)

	badSpan := []byte(`{"TraceID": "123"asjlgajdfhilqghi[adfvca} bad json`)
	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&badSpan),
	}
	searchHits := &elastic.SearchHits{Hits: hits}

	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations), Hits: searchHits}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{
					{Hits: searchHits},
					{Hits: searchHits},
				},
			}, nil)

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.Error(t, err)
		assert.Len(t, traces, 0)
	})
}

func TestFindTraceIDs(t *testing.T) {
	testCases := []struct {
		aggregrationID string
	}{
		{traceIDAggregation},
		{servicesAggregation},
		{operationsAggregation},
	}
	for _, testCase := range testCases {
		t.Run(testCase.aggregrationID, func(t *testing.T) {
			testGet(testCase.aggregrationID, t)
		})
	}
}

func TestTraceIDsStringsToModelsConversion(t *testing.T) {
	traceIDs, err := convertTraceIDsStringsToModels([]string{"1", "2", "3"})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(traceIDs))
	assert.Equal(t, model.NewTraceID(0, 1), traceIDs[0])

	traceIDs, err = convertTraceIDsStringsToModels([]string{"dsfjsdklfjdsofdfsdbfkgbgoaemlrksdfbsdofgerjl"})
	assert.EqualError(t, err, "making traceID from string 'dsfjsdklfjdsofdfsdbfkgbgoaemlrksdfbsdofgerjl' failed: TraceID cannot be longer than 32 hex characters: dsfjsdklfjdsofdfsdbfkgbgoaemlrksdfbsdofgerjl")
	assert.Equal(t, 0, len(traceIDs))
}

func mockMultiSearchService(r *spanReaderTest) *mock.Call {
	multiSearchService := &mocks.MultiSearchService{}
	multiSearchService.On("Add", mock.Anything, mock.Anything, mock.Anything).Return(multiSearchService)
	multiSearchService.On("Index", mock.AnythingOfType("string"), mock.AnythingOfType("string"),
		mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(multiSearchService)
	r.client.On("MultiSearch").Return(multiSearchService)
	return multiSearchService.On("Do", mock.AnythingOfType("*context.valueCtx"))
}

func mockArchiveMultiSearchService(r *spanReaderTest, indexName string) *mock.Call {
	multiSearchService := &mocks.MultiSearchService{}
	multiSearchService.On("Add", mock.Anything, mock.Anything, mock.Anything).Return(multiSearchService)
	multiSearchService.On("Index", indexName).Return(multiSearchService)
	r.client.On("MultiSearch").Return(multiSearchService)
	return multiSearchService.On("Do", mock.AnythingOfType("*context.valueCtx"))
}

// matchTermsAggregation uses reflection to match the size attribute of the TermsAggregation; neither
// attributes nor getters are exported by TermsAggregation.
func matchTermsAggregation(termsAgg *elastic.TermsAggregation) bool {
	val := reflect.ValueOf(termsAgg).Elem()
	sizeVal := val.FieldByName("size").Elem().Int()
	return sizeVal == defaultMaxDocCount
}

func mockSearchService(r *spanReaderTest) *mock.Call {
	searchService := &mocks.SearchService{}
	searchService.On("Query", mock.Anything).Return(searchService)
	searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
	searchService.On("Size", mock.MatchedBy(func(size int) bool {
		return size == 0 // Aggregations apply size (bucket) limits in their own query objects, and do not apply at the parent query level.
	})).Return(searchService)
	searchService.On("Aggregation", stringMatcher(servicesAggregation), mock.MatchedBy(matchTermsAggregation)).Return(searchService)
	searchService.On("Aggregation", stringMatcher(operationsAggregation), mock.MatchedBy(matchTermsAggregation)).Return(searchService)
	searchService.On("Aggregation", stringMatcher(traceIDAggregation), mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
	r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)
	return searchService.On("Do", mock.MatchedBy(func(ctx context.Context) bool {
		t := reflect.TypeOf(ctx).String()
		return t == "*context.valueCtx" || t == "*context.emptyCtx"
	}))
}

func TestTraceQueryParameterValidation(t *testing.T) {
	var malformedtqp *spanstore.TraceQueryParameters
	err := validateQuery(malformedtqp)
	assert.EqualError(t, err, ErrMalformedRequestObject.Error())

	tqp := &spanstore.TraceQueryParameters{
		ServiceName: "",
		Tags: map[string]string{
			"hello": "world",
		},
	}
	err = validateQuery(tqp)
	assert.EqualError(t, err, ErrServiceNameNotSet.Error())

	tqp.ServiceName = serviceName

	tqp.StartTimeMin = time.Time{} //time.Unix(0,0) doesn't work because timezones
	tqp.StartTimeMax = time.Time{}
	err = validateQuery(tqp)
	assert.EqualError(t, err, ErrStartAndEndTimeNotSet.Error())

	tqp.StartTimeMin = time.Now()
	tqp.StartTimeMax = time.Now().Add(-1 * time.Hour)
	err = validateQuery(tqp)
	assert.EqualError(t, err, ErrStartTimeMinGreaterThanMax.Error())

	tqp.StartTimeMin = time.Now().Add(-1 * time.Hour)
	tqp.StartTimeMax = time.Now()
	err = validateQuery(tqp)
	assert.Nil(t, err)

	tqp.DurationMin = time.Hour
	tqp.DurationMax = time.Minute
	err = validateQuery(tqp)
	assert.EqualError(t, err, ErrDurationMinGreaterThanMax.Error())
}

func TestSpanReader_buildTraceIDAggregation(t *testing.T) {
	expectedStr := `{ "terms":{
            "field":"traceID",
            "size":123,
            "order":{
               "startTime":"desc"
            }
         },
         "aggregations": {
            "startTime" : { "max": {"field": "startTime"}}
         }}`
	withSpanReader(func(r *spanReaderTest) {
		traceIDAggregation := r.reader.buildTraceIDAggregation(123)
		actual, err := traceIDAggregation.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal([]byte(expectedStr), &expected)
		expected["terms"].(map[string]interface{})["size"] = 123
		expected["terms"].(map[string]interface{})["order"] = []interface{}{map[string]string{"startTime": "desc"}}
		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildFindTraceIDsQuery(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		traceQuery := &spanstore.TraceQueryParameters{
			DurationMin:   time.Second,
			DurationMax:   time.Second * 2,
			StartTimeMin:  time.Time{},
			StartTimeMax:  time.Time{}.Add(time.Second),
			ServiceName:   "s",
			OperationName: "o",
			Tags: map[string]string{
				"hello": "world",
			},
		}

		actualQuery := r.reader.buildFindTraceIDsQuery(traceQuery)
		actual, err := actualQuery.Source()
		require.NoError(t, err)
		expectedQuery := elastic.NewBoolQuery().
			Must(
				r.reader.buildDurationQuery(time.Second, time.Second*2),
				r.reader.buildStartTimeQuery(time.Time{}, time.Time{}.Add(time.Second)),
				r.reader.buildServiceNameQuery("s"),
				r.reader.buildOperationNameQuery("o"),
				r.reader.buildTagQuery("hello", "world"),
			)
		expected, err := expectedQuery.Source()
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func TestSpanReader_buildDurationQuery(t *testing.T) {
	expectedStr :=
		`{ "range":
			{ "duration": { "include_lower": true,
				        "include_upper": true,
				        "from": 1000000,
				        "to": 2000000 }
			}
		}`
	withSpanReader(func(r *spanReaderTest) {
		durationMin := time.Second
		durationMax := time.Second * 2
		durationQuery := r.reader.buildDurationQuery(durationMin, durationMax)
		actual, err := durationQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal([]byte(expectedStr), &expected)
		// We need to do this because we cannot process a json into uint64.
		expected["range"].(map[string]interface{})["duration"].(map[string]interface{})["from"] = model.DurationAsMicroseconds(durationMin)
		expected["range"].(map[string]interface{})["duration"].(map[string]interface{})["to"] = model.DurationAsMicroseconds(durationMax)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildStartTimeQuery(t *testing.T) {
	expectedStr :=
		`{ "range":
			{ "startTime": { "include_lower": true,
				         "include_upper": true,
				         "from": 1000000,
				         "to": 2000000 }
			}
		}`
	withSpanReader(func(r *spanReaderTest) {
		startTimeMin := time.Time{}.Add(time.Second)
		startTimeMax := time.Time{}.Add(2 * time.Second)
		durationQuery := r.reader.buildStartTimeQuery(startTimeMin, startTimeMax)
		actual, err := durationQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal([]byte(expectedStr), &expected)
		// We need to do this because we cannot process a json into uint64.
		expected["range"].(map[string]interface{})["startTime"].(map[string]interface{})["from"] = model.TimeAsEpochMicroseconds(startTimeMin)
		expected["range"].(map[string]interface{})["startTime"].(map[string]interface{})["to"] = model.TimeAsEpochMicroseconds(startTimeMax)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildServiceNameQuery(t *testing.T) {
	expectedStr := `{ "match": { "process.serviceName": { "query": "bat" }}}`
	withSpanReader(func(r *spanReaderTest) {
		serviceNameQuery := r.reader.buildServiceNameQuery("bat")
		actual, err := serviceNameQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal([]byte(expectedStr), &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildOperationNameQuery(t *testing.T) {
	expectedStr := `{ "match": { "operationName": { "query": "spook" }}}`
	withSpanReader(func(r *spanReaderTest) {
		operationNameQuery := r.reader.buildOperationNameQuery("spook")
		actual, err := operationNameQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal([]byte(expectedStr), &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildTagQuery(t *testing.T) {
	inStr, err := ioutil.ReadFile("fixtures/query_01.json")
	require.NoError(t, err)
	withSpanReader(func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat.foo", "spook")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal(inStr, &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildTagRegexQuery(t *testing.T) {
	inStr, err := ioutil.ReadFile("fixtures/query_02.json")
	require.NoError(t, err)
	withSpanReader(func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat.foo", "spo.*")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal(inStr, &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildTagRegexEscapedQuery(t *testing.T) {
	inStr, err := ioutil.ReadFile("fixtures/query_03.json")
	require.NoError(t, err)
	withSpanReader(func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat.foo", "spo\\*")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal(inStr, &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_GetEmptyIndex(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)

		traceQuery := &spanstore.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    2,
		}

		services, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestSpanReader_ArchiveTraces(t *testing.T) {
	withArchiveSpanReader(false, func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockArchiveMultiSearchService(r, "jaeger-span-archive").
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)

		trace, err := r.reader.GetTrace(context.Background(), model.TraceID{})
		require.Nil(t, trace)
		assert.EqualError(t, err, "trace not found")
	})
}

func TestSpanReader_ArchiveTraces_ReadAlias(t *testing.T) {
	withArchiveSpanReader(true, func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockArchiveMultiSearchService(r, "jaeger-span-archive-read").
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)

		trace, err := r.reader.GetTrace(context.Background(), model.TraceID{})
		require.Nil(t, trace)
		assert.EqualError(t, err, "trace not found")
	})
}

func TestConvertTraceIDsStringsToModels(t *testing.T) {
	ids, err := convertTraceIDsStringsToModels([]string{"1", "2", "01", "02", "001", "002"})
	require.NoError(t, err)
	assert.Equal(t, []model.TraceID{model.NewTraceID(0, 1), model.NewTraceID(0, 2)}, ids)
	_, err = convertTraceIDsStringsToModels([]string{"1", "2", "01", "02", "001", "002", "blah"})
	assert.Error(t, err)
}

func TestBuildTraceByIDQuery(t *testing.T) {
	uintMax := ^uint64(0)
	traceIDNoHigh := model.NewTraceID(0, 1)
	traceIDHigh := model.NewTraceID(1, 1)
	traceID := model.NewTraceID(uintMax, uintMax)
	tests := []struct {
		traceID model.TraceID
		query   elastic.Query
	}{
		{
			traceID: traceIDNoHigh,
			query: elastic.NewBoolQuery().Should(
				elastic.NewTermQuery(traceIDField, "0000000000000001").Boost(2),
				elastic.NewTermQuery(traceIDField, "1"),
			),
		},
		{
			traceID: traceIDHigh,
			query: elastic.NewBoolQuery().Should(
				elastic.NewTermQuery(traceIDField, "00000000000000010000000000000001").Boost(2),
				elastic.NewTermQuery(traceIDField, "10000000000000001"),
			),
		},
		{
			traceID: traceID,
			query:   elastic.NewTermQuery(traceIDField, "ffffffffffffffffffffffffffffffff"),
		},
	}
	for _, test := range tests {
		q := buildTraceByIDQuery(test.traceID)
		assert.Equal(t, test.query, q)
	}
}
