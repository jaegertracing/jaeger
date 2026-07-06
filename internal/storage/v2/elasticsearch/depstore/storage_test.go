// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/clientbuilder"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
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
				"jaeger-dependencies-read",
			},
		},
		{
			rotation: periodicRotation("", "2006-01-02"),
			lookback: 23 * time.Hour,
			indices: []string{
				"jaeger-dependencies-" + fixedTime.Format("2006-01-02"),
				"jaeger-dependencies-" + fixedTime.Add(-23*time.Hour).Format("2006-01-02"),
			},
		},
		{
			rotation: periodicRotation("", "2006-01-02"),
			lookback: 13 * time.Hour,
			indices: []string{
				"jaeger-dependencies-" + fixedTime.UTC().Format("2006-01-02"),
				"jaeger-dependencies-" + fixedTime.Add(-13*time.Hour).Format("2006-01-02"),
			},
		},
		{
			rotation: periodicRotation("foo:", "2006-01-02"),
			lookback: 1 * time.Hour,
			indices: []string{
				"foo:-jaeger-dependencies-" + fixedTime.Format("2006-01-02"),
			},
		},
		{
			rotation: periodicRotation("foo-", "2006-01-02"),
			lookback: 0,
			indices: []string{
				"foo-jaeger-dependencies-" + fixedTime.Format("2006-01-02"),
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
			writeIndex: "jaeger-dependencies-write",
		},
		{
			rotation:   periodicRotation("", "2006-01-02"),
			writeIndex: "jaeger-dependencies-" + fixedTime.Format("2006-01-02"),
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

// TestDependencyStoreRequestSnapshots freezes the exact wire format of the
// dependency read+write path over the current olivere client. Every supported
// version emits the same request, so snapshots collapse to a single all-versions
// file.

// newDataClient builds a real es.Client for the given backend version, pointed at
// the recording server. Version is set explicitly so no ping is issued, and the
// bulk processor only flushes on Close.
func newDataClient(t *testing.T, url string, version es.BackendVersion) es.Client {
	cfg := &config.Configuration{
		Servers:            []string{url},
		Version:            uint(version),
		DisableHealthCheck: true,
		LogLevel:           "info",
		BulkProcessing:     config.BulkProcessing{MaxBytes: -1},
	}
	client, err := clientbuilder.NewClient(context.Background(), cfg, zap.NewNop(), metrics.NullFactory, nil)
	require.NoError(t, err)
	return client
}

// dataRecorder answers each request with an empty-but-valid response for its
// endpoint (search or bulk), so operations complete without error while the
// request is captured.
func dataRecorder() *snapshottest.Recorder {
	return snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "_bulk"):
			w.Write([]byte(`{"took":0,"errors":false,"items":[]}`))
		default:
			w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]}}`))
		}
	})
}

func TestDependencyStoreRequestSnapshots(t *testing.T) {
	fixedTime := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	const lookback = time.Hour
	dependencies := []dbmodel.DependencyLink{
		{Parent: "svcA", Child: "svcB", CallCount: 1},
	}

	getDependencies := map[es.BackendVersion]string{}
	writeDependencies := map[es.BackendVersion]string{}

	for _, version := range es.AllVersions {
		rec := dataRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)
		client := newDataClient(t, server.URL, version)
		// The test closes the client inline (below) to flush the bulk request; this
		// cleanup is a fallback for when an earlier assertion aborts first, so the
		// client is never closed twice.
		clientClosed := false
		t.Cleanup(func() {
			if !clientClosed {
				_ = client.Close()
			}
		})
		store := NewDependencyStore(Params{
			Client:      func() es.Client { return client },
			Logger:      zap.NewNop(),
			MaxDocCount: defaultMaxDocCount,
			Rotation:    periodicRotation("", "2006-01-02"),
		})
		ctx := context.Background()

		rec.Reset()
		_, err := store.GetDependencies(ctx, fixedTime, lookback)
		require.NoError(t, err)
		getDependencies[version] = rec.Marshal(t)

		rec.Reset()
		require.NoError(t, store.WriteDependencies(fixedTime, dependencies))
		require.NoError(t, client.Close()) // flushes the bulk request
		clientClosed = true
		writeDependencies[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/get_dependencies", getDependencies)
	snapshottest.AssertByVersion(t, "testdata/write_dependencies", writeDependencies)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
