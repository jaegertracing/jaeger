// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"context"
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

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/es"
	"github.com/jaegertracing/jaeger/internal/storage/es/config"
	"github.com/jaegertracing/jaeger/internal/storage/es/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const defaultMaxDocCount = 10_000

type depStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStorage(indexPrefix config.IndexPrefix, indexDateLayout string, maxDocCount int, fn func(r *depStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &depStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage: NewDependencyStore(Params{
			Client:          func() es.Client { return client },
			Logger:          logger,
			IndexPrefix:     indexPrefix,
			IndexDateLayout: indexDateLayout,
			MaxDocCount:     maxDocCount,
		}),
	}
	fn(r)
}

var (
	_ dependencystore.Reader = &DependencyStore{} // check API conformance
	_ dependencystore.Writer = &DependencyStore{} // check API conformance
)

func TestNewSpanReaderIndexPrefix(t *testing.T) {
	testCases := []struct {
		prefix   config.IndexPrefix
		expected string
	}{
		{prefix: "", expected: ""},
		{prefix: "foo", expected: "foo-"},
		{prefix: ":", expected: ":-"},
	}
	for _, testCase := range testCases {
		client := &mocks.Client{}
		r := NewDependencyStore(Params{
			Client:          func() es.Client { return client },
			Logger:          zap.NewNop(),
			IndexPrefix:     testCase.prefix,
			IndexDateLayout: "2006-01-02",
			MaxDocCount:     defaultMaxDocCount,
		})

		assert.Equal(t, testCase.expected+dependencyIndexBaseName, r.dependencyIndexPrefix)
	}
}

func TestWriteDependencies(t *testing.T) {
	testCases := []struct {
		writeError    error
		expectedError string
		esVersion     uint
	}{
		{
			expectedError: "",
			esVersion:     7,
		},
	}
	for _, testCase := range testCases {
		withDepStorage("", "2006-01-02", defaultMaxDocCount, func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
			indexName := indexWithDate("", "2006-01-02", fixedTime)
			writeService := &mocks.IndexService{}

			r.client.On("Index").Return(writeService)
			r.client.On("GetVersion").Return(testCase.esVersion)

			writeService.On("Index", stringMatcher(indexName)).Return(writeService)
			writeService.On("Type", stringMatcher(dependencyType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Add", mock.Anything).Return(nil, testCase.writeError)
			err := r.storage.WriteDependencies(fixedTime, []model.DependencyLink{})
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetDependencies(t *testing.T) {
	goodDependencies := `{
			"ts": 798434479000000,
			"dependencies": [
				{ "parent": "hello",
				  "child": "world",
				  "callCount": 12
				}
			]
		}`
	badDependencies := `badJson{hello}world`

	testCases := []struct {
		searchResult   *elastic.SearchResult
		searchError    error
		expectedError  string
		expectedOutput []model.DependencyLink
		indexPrefix    config.IndexPrefix
		maxDocCount    int
		indices        []any
	}{
		{
			searchResult: createSearchResult(goodDependencies),
			expectedOutput: []model.DependencyLink{
				{
					Parent:    "hello",
					Child:     "world",
					CallCount: 12,
				},
			},
			indices:     []any{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
			maxDocCount: 1000, // can be anything, assertion will check this value is used in search query.
		},
		{
			searchResult:  createSearchResult(badDependencies),
			expectedError: "unmarshalling ElasticSearch documents failed",
			indices:       []any{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for dependencies: search failure",
			indices:       []any{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for dependencies: search failure",
			indexPrefix:   "foo",
			indices:       []any{"foo-jaeger-dependencies-1995-04-21", "foo-jaeger-dependencies-1995-04-20"},
		},
	}
	for _, testCase := range testCases {
		withDepStorage(testCase.indexPrefix, "2006-01-02", testCase.maxDocCount, func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)

			searchService := &mocks.SearchService{}
			r.client.On("Search", testCase.indices...).Return(searchService)

			searchService.On("Size", mock.MatchedBy(func(size int) bool {
				return size == testCase.maxDocCount
			})).Return(searchService)
			searchService.On("Query", mock.Anything).Return(searchService)
			searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
			searchService.On("Do", mock.Anything).Return(testCase.searchResult, testCase.searchError)

			actual, err := r.storage.GetDependencies(context.Background(), fixedTime, 24*time.Hour)
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

func createSearchResult(dependencyLink string) *elastic.SearchResult {
	dependencyLinkRaw := []byte(dependencyLink)
	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: (*json.RawMessage)(&dependencyLinkRaw),
	}
	searchResult := &elastic.SearchResult{Hits: &elastic.SearchHits{Hits: hits}}
	return searchResult
}

func TestGetReadIndices(t *testing.T) {
	fixedTime := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	testCases := []struct {
		indices  []string
		lookback time.Duration
		params   Params
	}{
		{
			params:   Params{IndexPrefix: "", IndexDateLayout: "2006-01-02", UseReadWriteAliases: true},
			lookback: 23 * time.Hour,
			indices: []string{
				dependencyIndexBaseName + "read",
			},
		},
		{
			params:   Params{IndexPrefix: "", IndexDateLayout: "2006-01-02"},
			lookback: 23 * time.Hour,
			indices: []string{
				dependencyIndexBaseName + fixedTime.Format("2006-01-02"),
				dependencyIndexBaseName + fixedTime.Add(-23*time.Hour).Format("2006-01-02"),
			},
		},
		{
			params:   Params{IndexPrefix: "", IndexDateLayout: "2006-01-02"},
			lookback: 13 * time.Hour,
			indices: []string{
				dependencyIndexBaseName + fixedTime.UTC().Format("2006-01-02"),
				dependencyIndexBaseName + fixedTime.Add(-13*time.Hour).Format("2006-01-02"),
			},
		},
		{
			params:   Params{IndexPrefix: "foo:", IndexDateLayout: "2006-01-02"},
			lookback: 1 * time.Hour,
			indices: []string{
				"foo:" + config.IndexPrefixSeparator + dependencyIndexBaseName + fixedTime.Format("2006-01-02"),
			},
		},
		{
			params:   Params{IndexPrefix: "foo-", IndexDateLayout: "2006-01-02"},
			lookback: 0,
			indices: []string{
				"foo" + config.IndexPrefixSeparator + dependencyIndexBaseName + fixedTime.Format("2006-01-02"),
			},
		},
	}
	for _, testCase := range testCases {
		s := NewDependencyStore(testCase.params)
		assert.EqualValues(t, testCase.indices, s.getReadIndices(fixedTime, testCase.lookback))
	}
}

func TestGetWriteIndex(t *testing.T) {
	fixedTime := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	testCases := []struct {
		writeIndex string
		lookback   time.Duration
		params     Params
	}{
		{
			params:     Params{IndexPrefix: "", IndexDateLayout: "2006-01-02", UseReadWriteAliases: true},
			writeIndex: dependencyIndexBaseName + "write",
		},
		{
			params:     Params{IndexPrefix: "", IndexDateLayout: "2006-01-02", UseReadWriteAliases: false},
			writeIndex: dependencyIndexBaseName + fixedTime.Format("2006-01-02"),
		},
	}
	for _, testCase := range testCases {
		s := NewDependencyStore(testCase.params)
		assert.EqualValues(t, testCase.writeIndex, s.getWriteIndex(fixedTime))
	}
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) any {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
