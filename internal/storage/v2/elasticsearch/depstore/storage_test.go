// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esclientmocks "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const defaultMaxDocCount = 10_000

func periodicRotation(prefix config.IndexPrefix, dateLayout string) indices.Rotation {
	return indices.NewPeriodicRotation(
		prefix.Apply(config.DependencyIndexName),
		dateLayout,
		config.RolloverFrequencyDuration("day"),
	)
}

// hitsResponse builds an owned SearchResponse whose hits carry the given raw
// _source JSON documents, so the read-path parsing can be exercised without a
// live backend.
func hitsResponse(sources ...string) *esclient.SearchResponse {
	hits := make([]esclient.SearchHit, len(sources))
	for i, src := range sources {
		hits[i] = esclient.SearchHit{Source: json.RawMessage(src)}
	}
	return &esclient.SearchResponse{Hits: esclient.HitsResult{Hits: hits}}
}

var _ CoreDependencyStore = &DependencyStore{} // check API conformance

func TestWriteDependencies(t *testing.T) {
	bulkWriter := esclientmocks.NewBulkWriter(t)
	var added []esclient.BulkItem
	bulkWriter.On("Add", mock.Anything).Run(func(args mock.Arguments) {
		added = append(added, args.Get(0).(esclient.BulkItem))
	}).Return()
	store := NewDependencyStore(Params{
		BulkWriter:  bulkWriter,
		Logger:      zap.NewNop(),
		MaxDocCount: defaultMaxDocCount,
		Rotation:    periodicRotation("", "2006-01-02"),
	})

	fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
	dependencies := []dbmodel.DependencyLink{{Parent: "svcA", Child: "svcB", CallCount: 1}}
	require.NoError(t, store.WriteDependencies(fixedTime, dependencies))

	require.Len(t, added, 1)
	assert.Equal(t, "jaeger-dependencies-1995-04-21", added[0].Index)
	body := added[0].Body.(*dbmodel.TimeDependencies)
	assert.Equal(t, fixedTime, body.Timestamp)
	assert.Equal(t, dependencies, body.Dependencies)
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
		name           string
		searchResult   *esclient.SearchResponse
		searchError    error
		expectedError  string
		expectedOutput []dbmodel.DependencyLink
	}{
		{
			name:         "good dependencies",
			searchResult: hitsResponse(goodDependencies),
			expectedOutput: []dbmodel.DependencyLink{
				{Parent: "hello", Child: "world", CallCount: 12},
			},
		},
		{
			name:          "bad dependencies",
			searchResult:  hitsResponse(badDependencies),
			expectedError: "unmarshalling ElasticSearch documents failed",
		},
		{
			name:          "search failure",
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for dependencies: search failure",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			searcher := esclientmocks.NewSearcher(t)
			searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).
				Return(testCase.searchResult, testCase.searchError)
			store := NewDependencyStore(Params{
				Searcher:    searcher,
				Logger:      zap.NewNop(),
				MaxDocCount: 1000,
				Rotation:    periodicRotation("", "2006-01-02"),
			})

			fixedTime := time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC)
			actual, err := store.GetDependencies(context.Background(), fixedTime, 24*time.Hour)
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

// dataRecorder answers each request with an empty-but-valid response for its
// endpoint (search or bulk), so operations complete without error while the
// request is captured.
func dataRecorder() *snapshottest.Recorder {
	return snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "_bulk") {
			w.Write([]byte(`{"took":0,"errors":false,"items":[]}`))
			return
		}
		w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]}}`))
	})
}

// TestDependencyStoreRequestSnapshots asserts the wire format of the dependency
// read and write paths over esclient: get_dependencies (a _search with a timestamp
// range) and write_dependencies (a bulk write). Every supported version emits the
// same request, so the snapshots collapse to a single all-versions file.
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

		// One real esclient over the recording server backs both the search path and
		// the bulk writer. Version is pinned so the client skips its probe; the single
		// document buffers until Close, which flushes the one bulk request we record.
		esCfg := &config.Configuration{Servers: []string{server.URL}, Version: uint(version)}
		esClient, err := esclient.NewClient(context.Background(), esCfg, zap.NewNop(), nil)
		require.NoError(t, err)
		bulkWriter, err := esclient.NewBulkIndexer(esClient, esclient.BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)
		store := NewDependencyStore(Params{
			Searcher:    esclient.SearchClient{Client: esClient},
			BulkWriter:  bulkWriter,
			Logger:      zap.NewNop(),
			MaxDocCount: defaultMaxDocCount,
			Rotation:    periodicRotation("", "2006-01-02"),
		})
		ctx := context.Background()

		rec.Reset()
		_, err = store.GetDependencies(ctx, fixedTime, lookback)
		require.NoError(t, err)
		getDependencies[version] = rec.Marshal(t)

		rec.Reset()
		require.NoError(t, store.WriteDependencies(fixedTime, dependencies))
		require.NoError(t, bulkWriter.Close()) // flushes the bulk request
		writeDependencies[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/get_dependencies", getDependencies)
	snapshottest.AssertByVersion(t, "testdata/write_dependencies", writeDependencies)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
