// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package spanstore

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/storage/spanstore"
)

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
		reader:    NewSpanReader(client, logger),
	}
	fn(r)
}

func TestNewSpanReader(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		var reader spanstore.Reader = r.reader // check API conformance
		assert.NotNil(t, reader)
	})
}

func TestSpanReader_GetTrace(t *testing.T) {
	// TODO: write test once done with function
	// currently not doing anything, only for code coverage, ignore for code review
	withSpanReader(func(r *spanReaderTest) {
		s, e := r.reader.GetTrace(model.TraceID{})
		assert.Nil(t, s)
		assert.Nil(t, e)
	})
}

func TestSpanReader_findIndicesEmptyQuery(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		mockExistsService(r)

		actual := r.reader.findIndices(spanstore.TraceQueryParameters{})

		today := time.Now()
		yesterday := today.AddDate(0, 0, -1)
		twoDaysAgo := today.AddDate(0, 0, -2)

		expected := []string{
			r.reader.indexWithDate(today),
			r.reader.indexWithDate(yesterday),
			r.reader.indexWithDate(twoDaysAgo),
		}

		assert.EqualValues(t, expected, actual)
	})
}

// TODO: Dry the below two separate findIndices test
func TestSpanReader_findIndices(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		mockExistsService(r)

		actual := r.reader.findIndices(spanstore.TraceQueryParameters{
			StartTimeMin: time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC),
			StartTimeMax: time.Date(2017, time.April, 21, 4, 21, 19, 95, time.UTC),
		})

		var expected []string

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_findIndices2(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		mockExistsService(r)

		today := time.Now()

		actual := r.reader.findIndices(spanstore.TraceQueryParameters{
			StartTimeMin: today.AddDate(0, 0, -7),
			StartTimeMax: today.AddDate(0, 0, -1),
		})

		yesterday := today.AddDate(0, 0, -1)
		twoDaysAgo := today.AddDate(0, 0, -2)

		expected := []string{
			r.reader.indexWithDate(yesterday),
			r.reader.indexWithDate(twoDaysAgo),
		}

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_indexWithDate(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		actual := r.reader.indexWithDate(time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC))
		assert.Equal(t, "jaeger-1995-04-21", actual)
	})
}

func TestSpanReader_GetServices(t *testing.T) {
	testGet("services", t)
}

func TestSpanReader_getServicesAggregation(t *testing.T) {
	// this aggregation marshalled should look like this:
	//
	// "terms":{
	//    "size":3000,
	//    "field":"serviceName"
	// }
	withSpanReader(func(r *spanReaderTest) {
		serviceAggregation := r.reader.getServicesAggregation()
		actual, err := serviceAggregation.Source()
		require.NoError(t, err)
		expected := make(map[string]interface{})
		terms := make(map[string]interface{})
		expected["terms"] = terms
		terms["size"] = defaultDocCount
		terms["field"] = "serviceName"

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_GetOperations(t *testing.T) {
	testGet("operations", t)
}

func testGet(typ string, t *testing.T) {
	var aggregationType string
	if typ == "services" {
		aggregationType = servicesAggregation
	} else if typ == "operations" {
		aggregationType = operationsAggregation
	}
	goodAggregations := make(map[string]*json.RawMessage)
	rawMessage := []byte(`{"buckets": [{"key": "hello","doc_count": 16}]}`)
	goodAggregations[aggregationType] = (*json.RawMessage)(&rawMessage)

	badAggregations := make(map[string]*json.RawMessage)
	badRawMessage := []byte(`{"buckets": [{bad json]}asdf`)
	badAggregations[aggregationType] = (*json.RawMessage)(&badRawMessage)

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
			expectedOutput: []string{"hello"},
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
				mockExistsService(r)

				mockSearchService(typ, r).
					On("Do", mock.AnythingOfType("*context.emptyCtx")).
					Return(testCase.searchResult, testCase.searchError)

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
	if typ == "services" {
		return r.reader.GetServices()
	} else if typ == "operations" {
		return r.reader.GetOperations("someService")
	} else {
		return nil, errors.New("Specify services or operations only")
	}
}

func TestSpanReader_bucketToStringArray(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		buckets := make([]*elastic.AggregationBucketKeyItem, 3)
		buckets[0] = &elastic.AggregationBucketKeyItem{Key: "hello"}
		buckets[1] = &elastic.AggregationBucketKeyItem{Key: "world"}
		buckets[2] = &elastic.AggregationBucketKeyItem{Key: "2"}

		actual, err := r.reader.bucketToStringArray(buckets)
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

		_, err := r.reader.bucketToStringArray(buckets)
		assert.EqualError(t, err, "Non-string key found in aggregation")
	})
}

func TestSpanReader_FindTraces(t *testing.T) {
	// TODO: write test once done with function
	// currently not doing anything, only for code coverage, ignore for code review
	withSpanReader(func(r *spanReaderTest) {
		s, e := r.reader.FindTraces(nil)
		assert.Nil(t, s)
		assert.Nil(t, e)
	})
}

func mockExistsService(r *spanReaderTest) {
	existsService := &mocks.IndicesExistsService{}
	existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
	r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)
}

func mockSearchService(servicesOrOperations string, r *spanReaderTest) *mocks.SearchService {
	searchService := &mocks.SearchService{}
	searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
	searchService.On("Size", mock.MatchedBy(func(i int) bool {
		return i == 0
	})).Return(searchService)
	if servicesOrOperations == "services" {
		searchService.On("Aggregation",
			stringMatcher(servicesAggregation),
			mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
	} else if servicesOrOperations == "operations" {
		searchService.On("Aggregation",
			stringMatcher(operationsAggregation),
			mock.AnythingOfType("*elastic.FilterAggregation")).Return(searchService)
	}
	r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)
	return searchService
}
