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
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

type depStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStorage(indexPrefix string, fn func(r *depStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &depStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage:   NewDependencyStore(client, logger, indexPrefix),
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
		r := NewDependencyStore(client, zap.NewNop(), testCase.prefix)
		assert.Equal(t, testCase.expected+dependencyIndex+"*", r.index)
	}
}

func TestWriteDependencies(t *testing.T) {
	testCases := []struct {
		createIndexError error
		writeError       error
		expectedError    string
		esVersion        uint
	}{
		{
			createIndexError: errors.New("index not created"),
			expectedError:    "Failed to create index: index not created",
			esVersion:        6,
		},
		{
			createIndexError: errors.New("index not created"),
			expectedError:    "Failed to create index: index not created",
			esVersion:        7,
		},
	}
	for _, testCase := range testCases {
		withDepStorage("", func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
			indexName := indexWithDate("", fixedTime)

			indexService := &mocks.IndicesCreateService{}
			writeService := &mocks.IndexService{}
			r.client.On("Index").Return(writeService)
			r.client.On("GetVersion").Return(testCase.esVersion)
			r.client.On("CreateIndex", stringMatcher(indexName)).Return(indexService)

			if testCase.esVersion == 7 {
				indexService.On("Body", stringMatcher(dependenciesMapping7)).Return(indexService)
			} else {
				indexService.On("Body", stringMatcher(dependenciesMapping)).Return(indexService)
			}
			indexService.On("Do", mock.Anything).Return(nil, testCase.createIndexError)

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
		index          string
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
			index: "jaeger-dependencies-*",
		},
		{
			searchResult:  createSearchResult(badDependencies),
			expectedError: "Unmarshalling ElasticSearch documents failed",
			index:         "jaeger-dependencies-*",
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "Failed to search for dependencies: search failure",
			index:         "jaeger-dependencies-*",
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "Failed to search for dependencies: search failure",
			indexPrefix:   "foo",
			index:         "foo-jaeger-dependencies-*",
		},
	}
	for _, testCase := range testCases {
		withDepStorage(testCase.indexPrefix, func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)

			searchService := &mocks.SearchService{}
			r.client.On("Search", testCase.index).Return(searchService)

			searchService.On("Size", mock.Anything).Return(searchService)
			searchService.On("Query", mock.Anything).Return(searchService)
			searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
			searchService.On("Do", mock.Anything).Return(testCase.searchResult, testCase.searchError)

			actual, err := r.storage.GetDependencies(fixedTime, 24*time.Hour)
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

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
