// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
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
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const defaultMaxDocCount = 10_000

type depStorageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStorage(rotation indices.Rotation, maxDocCount int, fn func(r *depStorageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &depStorageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage: NewDependencyStore(Params{
			Client:      func() es.Client { return client },
			Logger:      logger,
			MaxDocCount: maxDocCount,
			Rotation:    rotation,
		}),
	}
	fn(r)
}

func periodicRotation(prefix config.IndexPrefix, dateLayout string) indices.Rotation {
	return indices.NewPeriodicRotation(
		prefix.Apply(config.DependencyIndexName),
		dateLayout,
		config.RolloverFrequencyDuration("day"),
	)
}

var _ CoreDependencyStore = &DependencyStore{} // check API conformance

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
		rotation := periodicRotation("", "2006-01-02")
		withDepStorage(rotation, defaultMaxDocCount, func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
			expectedIndex := "jaeger-dependencies-1995-04-21"
			writeService := &mocks.IndexService{}

			r.client.On("Index").Return(writeService)
			r.client.On("GetVersion").Return(testCase.esVersion)

			writeService.On("Index", stringMatcher(expectedIndex)).Return(writeService)
			writeService.On("Type", stringMatcher(dependencyType)).Return(writeService)
			writeService.On("BodyJson", mock.Anything).Return(writeService)
			writeService.On("Add", mock.Anything).Return(nil, testCase.writeError)
			err := r.storage.WriteDependencies(fixedTime, []dbmodel.DependencyLink{})
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
		expectedOutput []dbmodel.DependencyLink
		indexPrefix    config.IndexPrefix
		maxDocCount    int
		indices        []any
	}{
		{
			searchResult: createSearchResult(goodDependencies),
			expectedOutput: []dbmodel.DependencyLink{
				{
					Parent:    "hello",
					Child:     "world",
					CallCount: 12,
				},
			},
			indices:     []any{"jaeger-dependencies-1995-04-21", "jaeger-dependencies-1995-04-20"},
			maxDocCount: 1000,
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
		rotation := periodicRotation(testCase.indexPrefix, "2006-01-02")
		withDepStorage(rotation, testCase.maxDocCount, func(r *depStorageTest) {
			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)

			searchService := &mocks.SearchService{}
			r.client.On("Search", mock.AnythingOfType("[]string")).Return(searchService)

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
				assert.Equal(t, testCase.expectedOutput, actual)
			}
		})
	}
}

func createSearchResult(dependencyLink string) *elastic.SearchResult {
	dependencyLinkRaw := []byte(dependencyLink)
	hits := make([]*elastic.SearchHit, 1)
	hits[0] = &elastic.SearchHit{
		Source: dependencyLinkRaw,
	}
	searchResult := &elastic.SearchResult{Hits: &elastic.SearchHits{Hits: hits}}
	return searchResult
}

func TestReadTargets(t *testing.T) {
	fixedTime := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	testCases := []struct {
		rotation indices.Rotation
		lookback time.Duration
		indices  []string
	}{
		{
			rotation: indices.NewAliasedRotation(config.DependencyIndexName+config.IndexSeparator+"write", config.DependencyIndexName+config.IndexSeparator+"read"),
			lookback: 23 * time.Hour,
			indices: []string{
				config.DependencyIndexName + config.IndexSeparator + "read",
			},
		},
		{
			rotation: periodicRotation("", "2006-01-02"),
			lookback: 23 * time.Hour,
			indices: []string{
				config.DependencyIndexName + config.IndexSeparator + fixedTime.Format("2006-01-02"),
				config.DependencyIndexName + config.IndexSeparator + fixedTime.Add(-23*time.Hour).Format("2006-01-02"),
			},
		},
		{
			rotation: periodicRotation("", "2006-01-02"),
			lookback: 13 * time.Hour,
			indices: []string{
				config.DependencyIndexName + config.IndexSeparator + fixedTime.UTC().Format("2006-01-02"),
				config.DependencyIndexName + config.IndexSeparator + fixedTime.Add(-13*time.Hour).Format("2006-01-02"),
			},
		},
		{
			rotation: periodicRotation("foo:", "2006-01-02"),
			lookback: 1 * time.Hour,
			indices: []string{
				"foo:" + config.IndexSeparator + config.DependencyIndexName + config.IndexSeparator + fixedTime.Format("2006-01-02"),
			},
		},
		{
			rotation: periodicRotation("foo-", "2006-01-02"),
			lookback: 0,
			indices: []string{
				"foo" + config.IndexSeparator + config.DependencyIndexName + config.IndexSeparator + fixedTime.Format("2006-01-02"),
			},
		},
	}
	for _, testCase := range testCases {
		actual := testCase.rotation.ReadTargets(fixedTime.Add(-testCase.lookback), fixedTime)
		assert.Equal(t, testCase.indices, actual)
	}
}

func TestWriteTarget(t *testing.T) {
	fixedTime := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	testCases := []struct {
		rotation   indices.Rotation
		writeIndex string
	}{
		{
			rotation:   indices.NewAliasedRotation(config.DependencyIndexName+config.IndexSeparator+"write", config.DependencyIndexName+config.IndexSeparator+"read"),
			writeIndex: config.DependencyIndexName + config.IndexSeparator + "write",
		},
		{
			rotation:   periodicRotation("", "2006-01-02"),
			writeIndex: config.DependencyIndexName + config.IndexSeparator + fixedTime.Format("2006-01-02"),
		},
	}
	for _, testCase := range testCases {
		actual := testCase.rotation.WriteTarget(fixedTime)
		assert.Equal(t, testCase.writeIndex, actual)
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
