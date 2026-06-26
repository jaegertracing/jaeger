// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	samplemodel "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const defaultMaxDocCount = 10_000

type samplingStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *SamplingStore
}

func withEsSampling(indexPrefix config.IndexPrefix, indexDateLayout string, maxDocCount int, fn func(w *samplingStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	w := &samplingStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage: NewSamplingStore(Params{
			Client:      func() es.Client { return client },
			Logger:      logger,
			MaxDocCount: maxDocCount,
			Rotation: indices.NewPeriodicRotation(
				indexPrefix.Apply("jaeger-sampling"),
				indexDateLayout,
				config.RolloverFrequencyDuration("day"),
			),
		}),
	}
	fn(w)
}

func TestGetLatestIndex(t *testing.T) {
	tests := []struct {
		name          string
		lookback      time.Duration
		expectedError string
		indexError    error
		indexExist    bool
	}{
		{
			name:       "with index",
			lookback:   24 * time.Hour,
			indexExist: true,
		},
		{
			name:          "without index",
			lookback:      72 * time.Hour,
			expectedError: "failed to find latest index",
			indexExist:    false,
		},
		{
			name:          "check index existence error",
			lookback:      24 * time.Hour,
			expectedError: "failed to check index existence: fail",
			indexExist:    true,
			indexError:    errors.New("fail"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &mocks.Client{}
			store := NewSamplingStore(Params{
				Client:      func() es.Client { return client },
				Logger:      zap.NewNop(),
				MaxDocCount: defaultMaxDocCount,
				Lookback:    test.lookback,
				Rotation: indices.NewPeriodicRotation(
					"jaeger-sampling",
					"2006-01-02",
					config.RolloverFrequencyDuration("day"),
				),
			})
			indexService := &mocks.IndicesExistsService{}
			client.On("IndexExists", mock.Anything).Return(indexService)
			indexService.On("Do", mock.Anything).Return(test.indexExist, test.indexError)

			if test.indexExist && test.indexError == nil {
				searchService := &mocks.SearchService{}
				client.On("Search", mock.AnythingOfType("[]string")).Return(searchService)
				searchService.On("Size", mock.Anything).Return(searchService)
				searchService.On("IgnoreUnavailable", true).Return(searchService)
				emptyResult := &elastic.SearchResult{Hits: &elastic.SearchHits{Hits: []*elastic.SearchHit{}}}
				searchService.On("Do", mock.Anything).Return(emptyResult, nil)
			}

			actual, err := store.GetLatestProbabilities()
			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
				assert.Nil(t, actual)
			} else {
				require.NoError(t, err)
				assert.Nil(t, actual)
			}
		})
	}
}

func TestInsertThroughput(t *testing.T) {
	test := struct {
		name          string
		expectedError string
	}{
		name:          "insert throughput",
		expectedError: "",
	}

	t.Run(test.name, func(t *testing.T) {
		withEsSampling("", "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
			throughputs := []*samplemodel.Throughput{
				{Service: "my-svc", Operation: "op"},
				{Service: "our-svc", Operation: "op2"},
			}
			fixedTime := time.Now()
			indexName := "jaeger-sampling-" + fixedTime.UTC().Format("2006-01-02")
			writeService := &mocks.IndexService{}
			w.client.On("Index").Return(writeService)
			writeService.On("Index", stringMatcher(indexName)).Return(writeService)
			writeService.On("Type", stringMatcher(throughputType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Add", mock.Anything)
			err := w.storage.InsertThroughput(throughputs)
			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	})
}

func TestInsertProbabilitiesAndQPS(t *testing.T) {
	test := struct {
		name          string
		expectedError string
	}{
		name:          "insert probabilities and qps",
		expectedError: "",
	}

	t.Run(test.name, func(t *testing.T) {
		withEsSampling("", "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
			pAQ := dbmodel.ProbabilitiesAndQPS{
				Hostname:      "dell11eg843d",
				Probabilities: samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
				QPS:           samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
			}
			fixedTime := time.Now()
			indexName := "jaeger-sampling-" + fixedTime.UTC().Format("2006-01-02")
			writeService := &mocks.IndexService{}
			w.client.On("Index").Return(writeService)
			writeService.On("Index", stringMatcher(indexName)).Return(writeService)
			writeService.On("Type", stringMatcher(probabilitiesType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Add", mock.Anything)
			err := w.storage.InsertProbabilitiesAndQPS(pAQ.Hostname, pAQ.Probabilities, pAQ.QPS)
			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	})
}

func TestGetThroughput(t *testing.T) {
	mockIndex := "jaeger-sampling-" + time.Now().UTC().Format("2006-01-02")
	goodThroughputs := `{
			"timestamp": "2024-02-08T12:00:00Z",
			"throughputs": {
					"Service": "my-svc",
					"Operation": "op",
					"Count": 10
			}
	}`
	tests := []struct {
		name           string
		searchResult   *elastic.SearchResult
		searchError    error
		expectedError  string
		expectedOutput []*samplemodel.Throughput
		indexPrefix    config.IndexPrefix
		maxDocCount    int
		index          string
	}{
		{
			name:         "good throughputs without prefix",
			searchResult: createSearchResult(goodThroughputs),
			expectedOutput: []*samplemodel.Throughput{
				{
					Service:   "my-svc",
					Operation: "op",
					Count:     10,
				},
			},
			index:       mockIndex,
			maxDocCount: 1000,
		},
		{
			name:         "good throughputs without prefix",
			searchResult: createSearchResult(goodThroughputs),
			expectedOutput: []*samplemodel.Throughput{
				{
					Service:   "my-svc",
					Operation: "op",
					Count:     10,
				},
			},
			index:       mockIndex,
			maxDocCount: 1000,
		},
		{
			name:         "good throughputs with prefix",
			searchResult: createSearchResult(goodThroughputs),
			expectedOutput: []*samplemodel.Throughput{
				{
					Service:   "my-svc",
					Operation: "op",
					Count:     10,
				},
			},
			index:       mockIndex,
			indexPrefix: "foo",
			maxDocCount: 1000,
		},
		{
			name:          "bad throughputs",
			searchResult:  createSearchResult(`badJson{hello}world`),
			expectedError: "unmarshalling documents failed: invalid character 'b' looking for beginning of value",
			index:         mockIndex,
		},
		{
			name:          "search fails",
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for throughputs: search failure",
			index:         mockIndex,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withEsSampling(test.indexPrefix, "2006-01-02", defaultMaxDocCount, func(w *samplingStorageTest) {
				searchService := &mocks.SearchService{}
				if test.indexPrefix != "" {
					test.indexPrefix += "-"
				}
				index := test.indexPrefix.Apply(test.index)
				w.client.On("Search", []string{index}).Return(searchService)
				searchService.On("Size", mock.Anything).Return(searchService)
				searchService.On("Query", mock.Anything).Return(searchService)
				searchService.On("IgnoreUnavailable", true).Return(searchService)
				searchService.On("Do", mock.Anything).Return(test.searchResult, test.searchError)

				actual, err := w.storage.GetThroughput(time.Now().Add(-time.Minute), time.Now())
				if test.expectedError != "" {
					require.EqualError(t, err, test.expectedError)
					assert.Nil(t, actual)
				} else {
					require.NoError(t, err)
					assert.Equal(t, test.expectedOutput, actual)
				}
			})
		})
	}
}

func TestGetLatestProbabilities(t *testing.T) {
	mockIndex := "jaeger-sampling-" + time.Now().UTC().Format("2006-01-02")
	goodProbabilities := `{
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
	tests := []struct {
		name           string
		searchResult   *elastic.SearchResult
		searchError    error
		expectedOutput samplemodel.ServiceOperationProbabilities
		expectedError  string
		maxDocCount    int
		index          string
		indexPresent   bool
		indexError     error
		indexPrefix    config.IndexPrefix
	}{
		{
			name:         "good probabilities without prefix",
			searchResult: createSearchResult(goodProbabilities),
			expectedOutput: samplemodel.ServiceOperationProbabilities{
				"new-srv": {
					"op": 0.1,
				},
			},
			index:        mockIndex,
			maxDocCount:  1000,
			indexPresent: true,
		},
		{
			name:         "good probabilities with prefix",
			searchResult: createSearchResult(goodProbabilities),
			expectedOutput: samplemodel.ServiceOperationProbabilities{
				"new-srv": {
					"op": 0.1,
				},
			},
			index:        mockIndex,
			maxDocCount:  1000,
			indexPresent: true,
			indexPrefix:  "foo",
		},
		{
			name:          "bad probabilities",
			searchResult:  createSearchResult(`badJson{hello}world`),
			expectedError: "unmarshalling documents failed: invalid character 'b' looking for beginning of value",
			index:         mockIndex,
			indexPresent:  true,
		},
		{
			name:          "search fail",
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for Latest Probabilities: search failure",
			index:         mockIndex,
			indexPresent:  true,
		},
		{
			name:          "index check fail",
			indexError:    errors.New("index check failure"),
			expectedError: "failed to get latest index: failed to check index existence: index check failure",
			index:         mockIndex,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &mocks.Client{}
			store := NewSamplingStore(Params{
				Client:      func() es.Client { return client },
				Logger:      zap.NewNop(),
				MaxDocCount: defaultMaxDocCount,
				Lookback:    72 * time.Hour,
				Rotation: indices.NewPeriodicRotation(
					test.indexPrefix.Apply("jaeger-sampling"),
					"2006-01-02",
					config.RolloverFrequencyDuration("day"),
				),
			})

			searchService := &mocks.SearchService{}
			index := test.indexPrefix.Apply(test.index)
			client.On("Search", []string{index}).Return(searchService)
			searchService.On("Size", mock.Anything).Return(searchService)
			searchService.On("IgnoreUnavailable", true).Return(searchService)
			searchService.On("Do", mock.Anything).Return(test.searchResult, test.searchError)

			indicesexistsservice := &mocks.IndicesExistsService{}
			client.On("IndexExists", index).Return(indicesexistsservice)
			indicesexistsservice.On("Do", mock.Anything).Return(test.indexPresent, test.indexError)

			actual, err := store.GetLatestProbabilities()
			if test.expectedError != "" {
				require.EqualError(t, err, test.expectedError)
				assert.Nil(t, actual)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedOutput, actual)
			}
		})
	}
}

func createSearchResult(rawJsonStr string) *elastic.SearchResult {
	rawJsonArr := []byte(rawJsonStr)
	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: rawJsonArr,
	}
	searchResult := &elastic.SearchResult{Hits: &elastic.SearchHits{Hits: hits}}
	return searchResult
}

func stringMatcher(q string) any {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
