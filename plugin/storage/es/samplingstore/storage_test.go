// Copyright (c) 2024 The Jaeger Authors.
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

package samplingstore

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	samplemodel "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es/samplingstore/dbmodel"
)

const defaultMaxDocCount = 10_000

type samplingStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *SamplingStore
}

func withEsSampling(indexPrefix, indexDateLayout string, maxDocCount int, fn func(w *samplingStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	w := &samplingStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage: NewSamplingStore(SamplingStoreParams{
			Client:          func() es.Client { return client },
			Logger:          logger,
			IndexPrefix:     indexPrefix,
			IndexDateLayout: indexDateLayout,
			MaxDocCount:     maxDocCount,
		}),
	}
	fn(w)
}

func TestNewIndexPrefix(t *testing.T) {
	testCases := []struct {
		prefix   string
		expected string
	}{
		{prefix: "", expected: ""},
		{prefix: "foo", expected: "foo-"},
		{prefix: ":", expected: ":-"},
	}
	for _, testCase := range testCases {
		client := &mocks.Client{}
		r := NewSamplingStore(SamplingStoreParams{
			Client:          func() es.Client { return client },
			Logger:          zap.NewNop(),
			IndexPrefix:     testCase.prefix,
			IndexDateLayout: "2006-01-02",
			MaxDocCount:     defaultMaxDocCount,
		})

		assert.Equal(t, testCase.expected+samplingIndex, r.samplingIndexPrefix)
	}
}

func TestGetReadIndices(t *testing.T) {
	testCases := []struct {
		start time.Time
		end   time.Time
	}{
		{
			start: time.Date(2024, time.February, 10, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2024, time.February, 12, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, testCase := range testCases {
		expectedIndices := []string{
			"prefix-jaeger-sampling-2024-02-12",
			"prefix-jaeger-sampling-2024-02-11",
			"prefix-jaeger-sampling-2024-02-10",
		}
		rollover := -time.Hour * 24
		indices := getReadIndices("prefix-jaeger-sampling-", "2006-01-02", testCase.start, testCase.end, rollover)
		assert.Equal(t, expectedIndices, indices)
	}
}

func TestGetLatestIndices(t *testing.T) {
	testCases := []struct {
		indexPrefix         string
		indexDateLayout     string
		maxDuration         time.Duration
		expectedIndices     []string
		expectedErrorSubstr string
		indexExist          bool
	}{
		{
			indexPrefix:         "",
			indexDateLayout:     "2006-01-02",
			maxDuration:         24 * time.Hour,
			expectedIndices:     []string{indexWithDate("", "2006-01-02", time.Now().UTC())},
			expectedErrorSubstr: "",
			indexExist:          true,
		},
		// Add more test cases as needed
	}

	for _, testCase := range testCases {
		withEsSampling(testCase.indexPrefix, testCase.indexDateLayout, defaultMaxDocCount, func(w *samplingStorageTest) {
			indexService := &mocks.IndicesExistsService{}
			indexName := indexWithDate(testCase.indexPrefix, testCase.indexDateLayout, time.Now().UTC())
			w.client.On("IndexExists", indexName).Return(indexService)
			indexService.On("Do", mock.Anything).Return(testCase.indexExist, nil)
			clientFnMock := w.storage.client()
			actualIndices, err := getLatestIndices(testCase.indexPrefix, testCase.indexDateLayout, clientFnMock, -24*time.Hour, testCase.maxDuration)
			if testCase.expectedErrorSubstr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.expectedErrorSubstr)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedIndices, actualIndices)
			}
		})
	}
}

func TestInsertThroughput(t *testing.T) {
	testCases := []struct {
		writeError    error
		expectedError string
		esVersion     uint
	}{
		{
			expectedError: "",
			esVersion:     6,
		},
		{
			expectedError: "",
			esVersion:     7,
		},
	}
	for _, testCase := range testCases {
		withEsSampling("", "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
			throughputs := []*samplemodel.Throughput{
				{Service: "my-svc", Operation: "op"},
				{Service: "our-svc", Operation: "op2"},
			}
			fixedTime := time.Now()
			indexName := indexWithDate("", "2006-01-02", fixedTime)
			writeService := &mocks.IndexService{}
			w.client.On("Index").Return(writeService)
			w.client.On("GetVersion").Return(testCase.esVersion)

			writeService.On("Index", stringMatcher(indexName)).Return(writeService)
			writeService.On("Type", stringMatcher(throughputType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Add", mock.Anything).Return(nil, testCase.writeError)
			err := w.storage.InsertThroughput(throughputs)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}

func TestInsertProbabilitiesAndQPS(t *testing.T) {
	testCases := []struct {
		writeError    error
		expectedError string
		esVersion     uint
	}{
		{
			expectedError: "",
			esVersion:     6,
		},
		{
			expectedError: "",
			esVersion:     7,
		},
	}
	for _, testCase := range testCases {
		withEsSampling("", "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
			pAQ := dbmodel.ProbabilitiesAndQPS{
				Hostname:      "dell11eg843d",
				Probabilities: samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
				QPS:           samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
			}
			fixedTime := time.Now()
			indexName := indexWithDate("", "2006-01-02", fixedTime)
			writeService := &mocks.IndexService{}
			w.client.On("Index").Return(writeService)
			w.client.On("GetVersion").Return(testCase.esVersion)

			writeService.On("Index", stringMatcher(indexName)).Return(writeService)
			writeService.On("Type", stringMatcher(probabilitiesType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Add", mock.Anything).Return(nil, testCase.writeError)
			err := w.storage.InsertProbabilitiesAndQPS(pAQ.Hostname, pAQ.Probabilities, pAQ.QPS)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetThroughput(t *testing.T) {
	mockIndex := "jaeger-sampling-" + time.Now().UTC().Format("2006-01-02")
	goodThroughputs := `{
		"timestamp": "2024-02-08T12:00:00Z",
		"throughputs": [
			{
				"Service": "my-svc",
				"Operation": "op",
				"Count": 10
			},
			{
				"Service": "another-svc",
				"Operation": "another-op",
				"Count": 20
			}
		]
	}`
	badThroughputs := `badJson{hello}world`
	testCases := []struct {
		searchResult   *elastic.SearchResult
		searchError    error
		expectedError  string
		expectedOutput []*samplemodel.Throughput
		indexPrefix    string
		maxDocCount    int
		indices        []interface{}
	}{
		{
			searchResult: createSearchResult(goodThroughputs),
			expectedOutput: []*samplemodel.Throughput{
				{
					Service:   "my-svc",
					Operation: "op",
					Count:     10,
				},
				{
					Service:   "another-svc",
					Operation: "another-op",
					Count:     20,
				},
			},
			indices:       []interface{}{mockIndex},
			maxDocCount:   1000,
			expectedError: "",
		},
		{
			searchResult:  createSearchResult(badThroughputs),
			expectedError: "unmarshalling documents failed: invalid character 'b' looking for beginning of value",
			indices:       []interface{}{mockIndex},
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for throughputs: search failure",
			indices:       []interface{}{mockIndex},
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for throughputs: search failure",
			indexPrefix:   "foo",
			indices:       []interface{}{mockIndex},
		},
	}
	for _, testCase := range testCases {
		withEsSampling("", "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
			searchService := &mocks.SearchService{}
			w.client.On("Search", testCase.indices...).Return(searchService)

			searchService.On("Size", mock.Anything).Return(searchService)
			searchService.On("Query", mock.Anything).Return(searchService)
			searchService.On("IgnoreUnavailable", true).Return(searchService)
			searchService.On("Do", mock.Anything).Return(testCase.searchResult, testCase.searchError)

			actual, err := w.storage.GetThroughput(time.Now().Add(-time.Minute), time.Now())
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
				assert.Nil(t, actual)
			} else {
				require.NoError(t, err)
				assert.EqualValues(t, testCase.expectedOutput, actual)
			}
		})
	}
}

func TestGetLatestProbabilities(t *testing.T) {
	mockIndex := "jaeger-sampling-" + time.Now().UTC().Format("2006-01-02")
	latestProbabilities := `{
		"timestamp": "2024-02-08T12:00:00Z",
		"probabilitiesandqps": {
			"Hostname": "dell11eg843d",
			"Probabilities": {
				"new-srv": {"op": 0.1}
			},
			"QPS": {
				"new-srv": {"op": 4}
			}
		}
	}`
	badProbabilities := `badJson{hello}world`
	testCases := []struct {
		searchResult   *elastic.SearchResult
		searchError    error
		expectedOutput samplemodel.ServiceOperationProbabilities
		expectedError  string
		maxDocCount    int
		indices        []interface{}
		indexPresent   bool
		indexError     error
	}{
		{
			searchResult: createSearchResult(latestProbabilities),
			expectedOutput: samplemodel.ServiceOperationProbabilities{
				"new-srv": {
					"op": 0.1,
				},
			},
			indices:      []interface{}{mockIndex},
			maxDocCount:  1000,
			indexPresent: true,
		},
		{
			searchResult:  createSearchResult(badProbabilities),
			expectedError: "unmarshalling documents failed: invalid character 'b' looking for beginning of value",
			indices:       []interface{}{mockIndex},
			indexPresent:  true,
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for Latest Probabilities: search failure",
			indices:       []interface{}{mockIndex},
			indexPresent:  true,
		},
	}
	for _, testCase := range testCases {
		withEsSampling("", "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
			searchService := &mocks.SearchService{}
			w.client.On("Search", testCase.indices...).Return(searchService)
			searchService.On("Size", mock.Anything).Return(searchService)
			searchService.On("IgnoreUnavailable", true).Return(searchService)
			searchService.On("Do", mock.Anything).Return(testCase.searchResult, testCase.searchError)

			indicesexistsservice := &mocks.IndicesExistsService{}
			w.client.On("IndexExists", testCase.indices...).Return(indicesexistsservice)
			indicesexistsservice.On("Do", mock.Anything).Return(testCase.indexPresent, testCase.indexError)

			actual, err := w.storage.GetLatestProbabilities()
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
				assert.Nil(t, actual)
			} else {
				require.NoError(t, err)
				assert.EqualValues(t, testCase.expectedOutput, actual)
			}
		})
	}
}

func createSearchResult(rawJsonStr string) *elastic.SearchResult {
	rawJsonArr := []byte(rawJsonStr)
	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&rawJsonArr),
	}
	searchResult := &elastic.SearchResult{Hits: &elastic.SearchHits{Hits: hits}}
	return searchResult
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
