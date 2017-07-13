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

package dependencystore

import (
	"encoding/json"
	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/storage/dependencystore"
	"go.uber.org/zap"
	"strings"
	"testing"
	"time"
)

type depStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStorage(fn func(r *depStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &depStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage:   NewDependencyStore(client, logger),
	}
	fn(r)
}

func TestNewStorage(t *testing.T) {
	withDepStorage(func(r *depStorageTest) {
		var reader dependencystore.Reader = r.storage // check API conformance
		var writer dependencystore.Writer = r.storage // check API conformance
		assert.NotNil(t, reader)
		assert.NotNil(t, writer)
	})
}

func TestWriteDependencies(t *testing.T) {
	testCases := []struct {
		createIndexError error
		writeError       error
		expectedError    string
	}{
		{
			createIndexError: errors.New("index not created"),
			expectedError:    "Failed to create index: index not created",
		},
		{
			writeError:    errors.New("write failed"),
			expectedError: "Failed to write dependencies: write failed",
		},
		{},
	}
	for _, testCase := range testCases {
		withDepStorage(func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
			indexName := indexName(fixedTime)

			indexService := &mocks.IndicesCreateService{}
			writeService := &mocks.IndexService{}
			r.client.On("Index").Return(writeService)
			r.client.On("CreateIndex", stringMatcher(indexName)).Return(indexService)

			indexService.On("Body", stringMatcher(dependenciesMapping)).Return(indexService)
			indexService.On("Do", mock.Anything).Return(nil, testCase.createIndexError)

			writeService.On("Index", stringMatcher(indexName)).Return(writeService)
			writeService.On("Type", stringMatcher(dependencyType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Do", mock.Anything).Return(nil, testCase.writeError)

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
		},
		{
			searchResult:  createSearchResult(badDependencies),
			expectedError: "Unmarshalling ElasticSearch documents failed",
		},
		{
			searchError:   errors.New("search failure"),
			expectedError: "Failed to search for dependencies: search failure",
		},
	}
	for _, testCase := range testCases {
		withDepStorage(func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
			indices := getIndices(fixedTime, 24*time.Hour)

			searchService := &mocks.SearchService{}
			r.client.On("Search", indices[0], indices[1]).Return(searchService)

			searchService.On("Type", stringMatcher(dependencyType)).Return(searchService)
			searchService.On("Size", mock.Anything).Return(searchService)
			searchService.On("Query", mock.Anything).Return(searchService)
			searchService.On("Do", mock.Anything).Return(testCase.searchResult, testCase.searchError)

			actual, err := r.storage.GetDependencies(fixedTime, 24*time.Hour)
			if testCase.expectedError != "" {
				assert.Nil(t, actual)
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.EqualValues(t, testCase.expectedOutput, actual)
				assert.NoError(t, err)
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
	searchHits := &elastic.SearchHits{Hits: hits}
	searchResult := &elastic.SearchResult{Hits: searchHits}
	return searchResult
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
