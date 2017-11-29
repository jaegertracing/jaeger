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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	"github.com/jaegertracing/jaeger/model"
	esJson "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/uber/jaeger-lib/metrics"
)

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
		reader:    newSpanReader(client, logger, 72*time.Hour),
	}
	fn(r)
}

var _ spanstore.Reader = &SpanReader{} // check API conformance

func TestNewSpanReader(t *testing.T) {
	client := &mocks.Client{}
	reader := NewSpanReader(client, zap.NewNop(), 0, metrics.NullFactory)
	assert.NotNil(t, reader)
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

		trace, err := r.reader.GetTrace(model.TraceID{Low: 1})
		require.NoError(t, err)
		require.NotNil(t, trace)

		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		require.Len(t, trace.Spans, 1)
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
		trace, err := r.reader.GetTrace(model.TraceID{Low: 1})
		require.EqualError(t, err, "No trace with that ID found")
		require.Nil(t, trace)
	})
}

func TestSpanReader_GetTraceNilHitsError(t *testing.T) {
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

		trace, err := r.reader.GetTrace(model.TraceID{Low: 1})
		require.EqualError(t, err, "No hits in read results found")
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

		trace, err := r.reader.GetTrace(model.TraceID{Low: 1})
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

		trace, err := r.reader.GetTrace(model.TraceID{Low: 1})
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

		var expectedSpan esJson.Span
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

	testCases := []struct {
		startTime time.Time
		endTime   time.Time
		expected  []string
	}{
		{
			startTime: today.Add(-time.Millisecond),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndexPrefix, today),
			},
		},
		{
			startTime: today.Add(-13 * time.Hour),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndexPrefix, today),
				indexWithDate(spanIndexPrefix, yesterday),
			},
		},
		{
			startTime: today.Add(-48 * time.Hour),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndexPrefix, today),
				indexWithDate(spanIndexPrefix, yesterday),
				indexWithDate(spanIndexPrefix, twoDaysAgo),
			},
		},
	}
	for _, testCase := range testCases {
		actual := findIndices(spanIndexPrefix, testCase.startTime, testCase.endTime)
		assert.EqualValues(t, testCase.expected, actual)
	}
}

func TestSpanReader_indexWithDate(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		actual := indexWithDate(spanIndexPrefix, time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC))
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
		expectedError  string
		expectedOutput []string
	}{
		{
			caption:        typ + " full behavior",
			searchResult:   &elastic.SearchResult{Aggregations: elastic.Aggregations(goodAggregations)},
			expectedOutput: []string{"123"},
		},
		{
			caption:       typ + " search error",
			searchError:   errors.New("Search failure"),
			expectedError: "Search service failed: Search failure",
		},
		{
			caption:       typ + " search error",
			searchResult:  &elastic.SearchResult{Aggregations: elastic.Aggregations(badAggregations)},
			expectedError: "Could not find aggregation of " + typ,
		},
	}
	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanReader(func(r *spanReaderTest) {
				mockSearchService(r).Return(testCase.searchResult, testCase.searchError)

				actual, err := returnSearchFunc(typ, r)
				if testCase.expectedError != "" {
					require.EqualError(t, err, testCase.expectedError)
					assert.Nil(t, actual)
				} else {
					require.NoError(t, err)
					assert.EqualValues(t, testCase.expectedOutput, actual)
				}
			})
		})
	}
}

func returnSearchFunc(typ string, r *spanReaderTest) ([]string, error) {
	if typ == servicesAggregation {
		return r.reader.GetServices()
	} else if typ == operationsAggregation {
		return r.reader.GetOperations("someService")
	} else if typ == traceIDAggregation {
		return r.reader.findTraceIDs(&spanstore.TraceQueryParameters{})
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
		assert.EqualError(t, err, "Non-string key found in aggregation")
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
			NumTraces:    2,
		}

		traces, err := r.reader.FindTraces(traceQuery)
		require.NoError(t, err)
		assert.Len(t, traces, 2)

		trace := traces[0]
		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		require.Len(t, trace.Spans, 1)
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

		traces, err := r.reader.FindTraces(traceQuery)
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

		traces, err := r.reader.FindTraces(traceQuery)
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

		traces, err := r.reader.FindTraces(traceQuery)
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

		traces, err := r.reader.FindTraces(traceQuery)
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

		traces, err := r.reader.FindTraces(traceQuery)
		require.Error(t, err)
		assert.Len(t, traces, 0)
	})
}

func TestFindTraceIDs(t *testing.T) {
	testGet(traceIDAggregation, t)
}

func mockMultiSearchService(r *spanReaderTest) *mock.Call {
	multiSearchService := &mocks.MultiSearchService{}
	multiSearchService.On("Add", mock.Anything, mock.Anything, mock.Anything).Return(multiSearchService)
	multiSearchService.On("Index", mock.AnythingOfType("string"), mock.AnythingOfType("string"),
		mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(multiSearchService)
	r.client.On("MultiSearch").Return(multiSearchService)
	return multiSearchService.On("Do", mock.AnythingOfType("*context.emptyCtx"))
}

func mockSearchService(r *spanReaderTest) *mock.Call {
	searchService := &mocks.SearchService{}
	searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
	searchService.On("Type", stringMatcher(spanType)).Return(searchService)
	searchService.On("Query", mock.Anything).Return(searchService)
	searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
	searchService.On("Size", mock.MatchedBy(func(i int) bool {
		return i == 0 || i == defaultDocCount
	})).Return(searchService)
	searchService.On("Aggregation", stringMatcher(servicesAggregation), mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
	searchService.On("Aggregation", stringMatcher(operationsAggregation), mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
	searchService.On("Aggregation", stringMatcher(traceIDAggregation), mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
	r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)
	return searchService.On("Do", mock.AnythingOfType("*context.emptyCtx"))
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
	expectedStr :=
		`{ "bool": {
		   "should": [
		      { "nested" : {
			 "path" : "tags",
			 "query" : {
			    "bool" : {
			      "must" : [
				 { "match" : {"tags.key" : {"query":"bat"}} },
				 { "match" : {"tags.value" : {"query":"spook"}} }
			      ]
		      }}}},
		      { "nested" : {
			 "path" : "process.tags",
			 "query" : {
			    "bool" : {
			      "must" : [
				 { "match" : {"process.tags.key" : {"query":"bat"}} },
				 { "match" : {"process.tags.value" : {"query":"spook"}} }
			      ]
		      }}}},
		      { "nested" : {
			 "path" : "logs.fields",
			 "query" : {
		            "bool" : {
			       "must" : [
			         { "match" : {"logs.fields.key" : {"query":"bat"}} },
			         { "match" : {"logs.fields.value" : {"query":"spook"}} }
			       ]
		      }}}}
		   ]
		}}`
	withSpanReader(func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat", "spook")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]interface{})
		json.Unmarshal([]byte(expectedStr), &expected)

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

		services, err := r.reader.FindTraces(traceQuery)
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}
