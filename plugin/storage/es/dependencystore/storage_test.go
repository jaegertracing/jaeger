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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

const defaultMaxDocCount = 10_000

type depStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStorage(indexPrefix, indexDateLayout string, maxDocCount int, fn func(r *depStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &depStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage:   NewDependencyStore(client, logger, indexPrefix, indexDateLayout, maxDocCount),
	}
	fn(r)
}

var _ dependencystore.Reader = &DependencyStore{} // check API conformance
var _ dependencystore.Writer = &DependencyStore{} // check API conformance

func TestNewSpanReaderIndexPrefix(t *testing.T) {
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
		r := NewDependencyStore(client, zap.NewNop(), testCase.prefix, "2006-01-02", defaultMaxDocCount)
		assert.Equal(t, testCase.expected+dependencyIndex, r.indexPrefix)
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
			esVersion:     6,
		},
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
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})

	}
}

func TestGetDependencies(t *testing.T) {
	goodDependencies :=
		`{
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
		indexPrefix    string
		maxDocCount    int
		indices        []interface{}
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
			indices:     []interface{}{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
			maxDocCount: 1000, // can be anything, assertion will check this value is used in search query.
		},
		{
			searchResult:  createSearchResult(badDependencies),
			expectedError: "unmarshalling ElasticSearch documents failed",
			indices:       []interface{}{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for dependencies: search failure",
			indices:       []interface{}{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for dependencies: search failure",
			indexPrefix:   "foo",
			indices:       []interface{}{"foo-jaeger-dependencies-1995-04-21", "foo-jaeger-dependencies-1995-04-20"},
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
				assert.EqualError(t, err, testCase.expectedError)
				assert.Nil(t, actual)
			} else {
				assert.NoError(t, err)
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

func TestGetIndices(t *testing.T) {
	fixedTime := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	testCases := []struct {
		expected []string
		lookback time.Duration
		prefix   string
	}{
		{
			expected: []string{indexWithDate("", "2006-01-02", fixedTime), indexWithDate("", "2006-01-02", fixedTime.Add(-24*time.Hour))},
			lookback: 23 * time.Hour,
			prefix:   "",
		},
		{
			expected: []string{indexWithDate("", "2006-01-02", fixedTime), indexWithDate("", "2006-01-02", fixedTime.Add(-24*time.Hour))},
			lookback: 13 * time.Hour,
			prefix:   "",
		},
		{
			expected: []string{indexWithDate("foo:", "2006-01-02", fixedTime)},
			lookback: 1 * time.Hour,
			prefix:   "foo:",
		},
		{
			expected: []string{indexWithDate("foo-", "2006-01-02", fixedTime)},
			lookback: 0,
			prefix:   "foo-",
		},
	}
	for _, testCase := range testCases {
		assert.EqualValues(t, testCase.expected, getIndices(testCase.prefix, "2006-01-02", fixedTime, testCase.lookback))
	}
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
