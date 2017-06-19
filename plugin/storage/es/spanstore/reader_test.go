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
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"encoding/json"
	"errors"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/storage/spanstore"
	"time"
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
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		actual := r.reader.findIndices(spanstore.TraceQueryParameters{})

		today := time.Now().Format("2006-01-02")
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		twoDaysAgo := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

		expected := []string{
			"jaeger-" + today,
			"jaeger-" + yesterday,
			"jaeger-" + twoDaysAgo,
		}

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_findIndices(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		actual := r.reader.findIndices(spanstore.TraceQueryParameters{
			StartTimeMin: time.Date(2017, time.April, 18, 4, 21, 19, 95, time.UTC),
			StartTimeMax: time.Date(2017, time.April, 21, 4, 21, 19, 95, time.UTC),
		})

		var expected []string

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_findIndices2(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		actual := r.reader.findIndices(spanstore.TraceQueryParameters{
			StartTimeMin: time.Now().AddDate(0, 0, -7),
			StartTimeMax: time.Now().AddDate(0, 0, -1),
		})

		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		twoDaysAgo := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

		expected := []string{
			"jaeger-" + yesterday,
			"jaeger-" + twoDaysAgo,
		}

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_GetServices(t *testing.T) {
	aggregations := make(map[string]*json.RawMessage)

	rawMessage := []byte(`{"buckets": [{"key": "hello","doc_count": 16}]}`)
	aggregations["distinct_services"] = (*json.RawMessage)(&rawMessage)

	testCase := struct {
		searchResult *elastic.SearchResult
		searchError  error
	}{
		searchResult: &elastic.SearchResult{
			Aggregations: elastic.Aggregations(aggregations),
		},
	}
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		searchService := &mocks.SearchService{}
		searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
		searchService.On("Size", mock.MatchedBy(func(i int) bool {
			return i == 0
		})).Return(searchService)
		searchService.On("Aggregation",
			stringMatcher("distinct_services"),
			mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)

		searchService.On("Do", mock.AnythingOfType("*context.emptyCtx")).
			Return(testCase.searchResult, testCase.searchError)

		actual, err := r.reader.GetServices()
		require.NoError(t, err)
		expected := []string{"hello"}
		assert.EqualValues(t, actual, expected)
	})
}

func TestSpanReader_GetServicesSearchError(t *testing.T) {
	//aggregations := make(map[string]*json.RawMessage)
	//
	//rawMessage := []byte(`{"buckets": [{"key": "hello","doc_count": 16}]}`)
	//aggregations["distinct_services"] = (*json.RawMessage)(&rawMessage)

	testCase := struct {
		searchResult *elastic.SearchResult
		searchError  error
	}{
		searchError: errors.New("Search failure"),
	}
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		searchService := &mocks.SearchService{}
		searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
		searchService.On("Size", mock.MatchedBy(func(i int) bool {
			return i == 0
		})).Return(searchService)
		searchService.On("Aggregation",
			stringMatcher("distinct_services"),
			mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)

		searchService.On("Do", mock.AnythingOfType("*context.emptyCtx")).
			Return(testCase.searchResult, testCase.searchError)

		actual, err := r.reader.GetServices()
		require.Error(t, err, "Search failure")
		assert.Nil(t, actual)
	})
}

func TestSpanReader_GetServicesAggregationError(t *testing.T) {
	aggregations := make(map[string]*json.RawMessage)

	rawMessage := []byte(`{"buckets": [{bad json]}asdf`)
	aggregations["distinct_services"] = (*json.RawMessage)(&rawMessage)

	testCase := struct {
		searchResult *elastic.SearchResult
		searchError  error
	}{
		searchResult: &elastic.SearchResult{
			Aggregations: elastic.Aggregations(aggregations),
		},
	}
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		searchService := &mocks.SearchService{}
		searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
		searchService.On("Size", mock.MatchedBy(func(i int) bool {
			return i == 0
		})).Return(searchService)
		searchService.On("Aggregation",
			stringMatcher("distinct_services"),
			mock.AnythingOfType("*elastic.TermsAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)

		searchService.On("Do", mock.AnythingOfType("*context.emptyCtx")).
			Return(testCase.searchResult, testCase.searchError)

		actual, err := r.reader.GetServices()
		require.Error(t, err, "Could not find aggregation of services")
		assert.Nil(t, actual)
	})
}

func TestSpanReader_getServicesAggregation(t *testing.T) {
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
		terms["size"] = 3000
		terms["field"] = "serviceName"

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_GetOperations(t *testing.T) {
	aggregations := make(map[string]*json.RawMessage)

	rawMessage := []byte(`{"buckets": [{"key": "hello","doc_count": 16}]}`)
	aggregations["distinct_operations"] = (*json.RawMessage)(&rawMessage)

	testCase := struct {
		searchResult *elastic.SearchResult
		searchError  error
	}{
		searchResult: &elastic.SearchResult{
			Aggregations: elastic.Aggregations(aggregations),
		},
	}
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		searchService := &mocks.SearchService{}
		searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
		searchService.On("Size", mock.MatchedBy(func(i int) bool {
			return i == 0
		})).Return(searchService)
		searchService.On("Aggregation",
			stringMatcher("distinct_operations"),
			mock.AnythingOfType("*elastic.FilterAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)

		searchService.On("Do", mock.AnythingOfType("*context.emptyCtx")).
			Return(testCase.searchResult, testCase.searchError)

		actual, err := r.reader.GetOperations("someService")
		require.NoError(t, err)
		expected := []string{"hello"}
		assert.EqualValues(t, actual, expected)
	})
}

func TestSpanReader_GetOperationsSearchError(t *testing.T) {
	aggregations := make(map[string]*json.RawMessage)

	rawMessage := []byte(`{"buckets": [{"key": "hello","doc_count": 16}]}`)
	aggregations["distinct_operations"] = (*json.RawMessage)(&rawMessage)

	testCase := struct {
		searchResult *elastic.SearchResult
		searchError  error
	}{
		searchError: errors.New("Search failure"),
	}
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		searchService := &mocks.SearchService{}
		searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
		searchService.On("Size", mock.MatchedBy(func(i int) bool {
			return i == 0
		})).Return(searchService)
		searchService.On("Aggregation",
			stringMatcher("distinct_operations"),
			mock.AnythingOfType("*elastic.FilterAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)

		searchService.On("Do", mock.AnythingOfType("*context.emptyCtx")).
			Return(testCase.searchResult, testCase.searchError)

		actual, err := r.reader.GetOperations("someService")
		require.Error(t, err, "Search failure")
		assert.Nil(t, actual)
	})
}

func TestSpanReader_GetOperationsAggregationError(t *testing.T) {
	aggregations := make(map[string]*json.RawMessage)

	rawMessage := []byte(`{"buckets": [{"badJSON]}asgh`)
	aggregations["distinct_operations"] = (*json.RawMessage)(&rawMessage)

	testCase := struct {
		searchResult *elastic.SearchResult
		searchError  error
	}{
		searchResult: &elastic.SearchResult{
			Aggregations: elastic.Aggregations(aggregations),
		},
	}
	withSpanReader(func(r *spanReaderTest) {
		existsService := &mocks.IndicesExistsService{}
		existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(true, nil)
		r.client.On("IndexExists", mock.AnythingOfType("string")).Return(existsService)

		searchService := &mocks.SearchService{}
		searchService.On("Type", stringMatcher(serviceType)).Return(searchService)
		searchService.On("Size", mock.MatchedBy(func(i int) bool {
			return i == 0
		})).Return(searchService)
		searchService.On("Aggregation",
			stringMatcher("distinct_operations"),
			mock.AnythingOfType("*elastic.FilterAggregation")).Return(searchService)
		r.client.On("Search", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(searchService)

		searchService.On("Do", mock.AnythingOfType("*context.emptyCtx")).
			Return(testCase.searchResult, testCase.searchError)

		actual, err := r.reader.GetOperations("someService")
		require.Error(t, err, "Could not find aggregation of operations")
		assert.Nil(t, actual)
	})
}

func TestSpanReader_bucketToStringArray(t *testing.T) {
	withSpanReader(func(r *spanReaderTest) {
		buckets := make([]*elastic.AggregationBucketKeyItem, 3)
		buckets[0] = &elastic.AggregationBucketKeyItem{Key: "hello"}
		buckets[1] = &elastic.AggregationBucketKeyItem{Key: "world"}
		buckets[2] = &elastic.AggregationBucketKeyItem{Key: "2"}

		actual, err := r.reader.bucketToStringArray(buckets)
		require.NoError(t, err)

		expected := []string{"hello", "world", "2"}

		assert.EqualValues(t, expected, actual)
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
