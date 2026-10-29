// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

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
	samplemodel "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const defaultMaxDocCount = 10_000

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

func dailyRotation() indices.Rotation {
	return indices.NewPeriodicRotation(
		config.SamplingIndexName,
		"2006-01-02",
		config.RolloverFrequencyDuration("day"),
	)
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
			indexClient := esclientmocks.NewIndexAPI(t)
			indexClient.On("IndexExists", mock.Anything, mock.Anything).
				Return(test.indexExist, test.indexError)
			store := NewSamplingStore(Params{
				IndexClient: indexClient,
				Logger:      zap.NewNop(),
				MaxDocCount: defaultMaxDocCount,
				Lookback:    test.lookback,
				Rotation:    dailyRotation(),
			})

			_, err := store.getLatestIndex(context.Background())
			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestInsertThroughput(t *testing.T) {
	batchWriter := esclientmocks.NewBatchWriter(t)
	var added []esclient.BulkItem
	batchWriter.On("WriteBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		added = append(added, args.Get(1).([]esclient.BulkItem)...)
	}).Return(nil)

	fixedTime := time.Date(2024, 2, 8, 12, 0, 0, 0, time.UTC)
	store := NewSamplingStore(Params{
		BatchWriter: batchWriter,
		Logger:      zap.NewNop(),
		MaxDocCount: defaultMaxDocCount,
		Rotation:    dailyRotation(),
	})
	store.now = func() time.Time { return fixedTime }

	throughputs := []*samplemodel.Throughput{
		{Service: "my-svc", Operation: "op"},
		{Service: "our-svc", Operation: "op2"},
	}
	require.NoError(t, store.InsertThroughput(throughputs))

	indexName := "jaeger-sampling-2024-02-08"
	require.Len(t, added, 2)
	assert.Equal(t, indexName, added[0].Index)
	body := added[0].Body.(*dbmodel.TimeThroughput)
	assert.Equal(t, fixedTime, body.Timestamp)
	assert.Equal(t, "my-svc", body.Throughput.Service)
}

func TestInsertProbabilitiesAndQPS(t *testing.T) {
	batchWriter := esclientmocks.NewBatchWriter(t)
	var added []esclient.BulkItem
	batchWriter.On("WriteBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		added = append(added, args.Get(1).([]esclient.BulkItem)...)
	}).Return(nil)

	fixedTime := time.Date(2024, 2, 8, 12, 0, 0, 0, time.UTC)
	store := NewSamplingStore(Params{
		BatchWriter: batchWriter,
		Logger:      zap.NewNop(),
		MaxDocCount: defaultMaxDocCount,
		Rotation:    dailyRotation(),
	})
	store.now = func() time.Time { return fixedTime }

	require.NoError(t, store.InsertProbabilitiesAndQPS(
		"dell11eg843d",
		samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
		samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
	))

	indexName := "jaeger-sampling-2024-02-08"
	require.Len(t, added, 1)
	assert.Equal(t, indexName, added[0].Index)
	body := added[0].Body.(*dbmodel.TimeProbabilitiesAndQPS)
	assert.Equal(t, fixedTime, body.Timestamp)
	assert.Equal(t, "dell11eg843d", body.ProbabilitiesAndQPS.Hostname)
}

func TestGetThroughput(t *testing.T) {
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
		searchResult   *esclient.SearchResponse
		searchError    error
		expectedError  string
		expectedOutput []*samplemodel.Throughput
	}{
		{
			name:         "good throughputs",
			searchResult: hitsResponse(goodThroughputs),
			expectedOutput: []*samplemodel.Throughput{
				{Service: "my-svc", Operation: "op", Count: 10},
			},
		},
		{
			name:          "bad throughputs",
			searchResult:  hitsResponse(`badJson{hello}world`),
			expectedError: "unmarshalling documents failed: invalid character 'b' looking for beginning of value",
		},
		{
			name:          "search fails",
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for throughputs: search failure",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			searcher := esclientmocks.NewSearcher(t)
			searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).
				Return(test.searchResult, test.searchError)
			store := NewSamplingStore(Params{
				Searcher:    searcher,
				Logger:      zap.NewNop(),
				MaxDocCount: 1000,
				Rotation:    dailyRotation(),
			})

			actual, err := store.GetThroughput(time.Now().Add(-time.Minute), time.Now())
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

func TestGetLatestProbabilities(t *testing.T) {
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
		searchResult   *esclient.SearchResponse
		searchError    error
		expectedOutput samplemodel.ServiceOperationProbabilities
		expectedError  string
		indexPresent   bool
		indexError     error
	}{
		{
			name:         "good probabilities",
			searchResult: hitsResponse(goodProbabilities),
			expectedOutput: samplemodel.ServiceOperationProbabilities{
				"new-srv": {"op": 0.1},
			},
			indexPresent: true,
		},
		{
			name:         "empty result",
			searchResult: hitsResponse(),
			indexPresent: true,
		},
		{
			name:          "bad probabilities",
			searchResult:  hitsResponse(`badJson{hello}world`),
			expectedError: "unmarshalling documents failed: invalid character 'b' looking for beginning of value",
			indexPresent:  true,
		},
		{
			name:          "search fail",
			searchError:   errors.New("search failure"),
			expectedError: "failed to search for Latest Probabilities: search failure",
			indexPresent:  true,
		},
		{
			name:          "index check fail",
			indexError:    errors.New("index check failure"),
			expectedError: "failed to get latest index: failed to check index existence: index check failure",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexClient := esclientmocks.NewIndexAPI(t)
			indexClient.On("IndexExists", mock.Anything, mock.Anything).
				Return(test.indexPresent, test.indexError)
			searcher := esclientmocks.NewSearcher(t)
			if test.indexPresent && test.indexError == nil {
				searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).
					Return(test.searchResult, test.searchError)
			}
			store := NewSamplingStore(Params{
				Searcher:    searcher,
				IndexClient: indexClient,
				Logger:      zap.NewNop(),
				MaxDocCount: 1000,
				Lookback:    72 * time.Hour,
				Rotation:    indices.NewAliasedRotation("jaeger-sampling-write", "jaeger-sampling-read"),
			})

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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

// samplingRecorder answers a HEAD index-exists check with 200 (so the index
// resolves), searches with an empty result, and _bulk writes with an empty item
// list, capturing each request.
func samplingRecorder() *snapshottest.Recorder {
	return snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "_bulk") {
			w.Write([]byte(`{"took":0,"errors":false,"items":[]}`))
			return
		}
		w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]}}`))
	})
}

// TestSamplingStoreRequestSnapshots freezes the wire format of the sampling
// read and write paths over esclient: get_throughput (a _search with a timestamp
// range), get_latest_probabilities (an index-exists HEAD followed by a _search),
// and the two bulk writes. A fixed clock (injected through the now field) stamps
// the write bodies so they are deterministic.
func TestSamplingStoreRequestSnapshots(t *testing.T) {
	start := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(time.Hour)
	writeTime := time.Date(2024, 2, 8, 12, 0, 0, 0, time.UTC)

	getThroughput := map[es.BackendVersion]string{}
	getLatestProbabilities := map[es.BackendVersion]string{}
	writeThroughput := map[es.BackendVersion]string{}
	writeProbabilities := map[es.BackendVersion]string{}

	for _, version := range es.AllVersions {
		rec := samplingRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)

		// One real esclient over the recording server backs the search, the bulk
		// writer, and the index-exists check. Version is pinned so the client skips
		// its probe; written documents buffer until Close flushes the bulk request.
		esCfg := &config.Configuration{Servers: []string{server.URL}, Version: uint(version)}
		esClient, err := esclient.NewClient(context.Background(), esCfg, zap.NewNop(), nil)
		require.NoError(t, err)
		store := NewSamplingStore(Params{
			Searcher:    esclient.SearchClient{Client: esClient},
			IndexClient: &esclient.IndicesClient{Client: esClient},
			Logger:      zap.NewNop(),
			MaxDocCount: 1000,
			Lookback:    72 * time.Hour,
			Rotation:    indices.NewAliasedRotation("jaeger-sampling-write", "jaeger-sampling-read"),
		})
		store.now = func() time.Time { return writeTime }

		rec.Reset()
		_, err = store.GetThroughput(start, end)
		require.NoError(t, err)
		getThroughput[version] = rec.Marshal(t)

		rec.Reset()
		_, err = store.GetLatestProbabilities()
		require.NoError(t, err)
		getLatestProbabilities[version] = rec.Marshal(t)

		writeThroughput[version] = captureBulkWrite(t, esClient, rec, store, func() {
			require.NoError(t, store.InsertThroughput([]*samplemodel.Throughput{
				{Service: "my-svc", Operation: "op", Count: 10},
			}))
		})
		writeProbabilities[version] = captureBulkWrite(t, esClient, rec, store, func() {
			require.NoError(t, store.InsertProbabilitiesAndQPS(
				"dell11eg843d",
				samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}},
				samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}},
			))
		})
	}

	snapshottest.AssertByVersion(t, "testdata/get_throughput", getThroughput)
	snapshottest.AssertByVersion(t, "testdata/get_latest_probabilities", getLatestProbabilities)
	snapshottest.AssertByVersion(t, "testdata/write_throughput", writeThroughput)
	snapshottest.AssertByVersion(t, "testdata/write_probabilities", writeProbabilities)
}

// captureBulkWrite gives the store a fresh bulk indexer, runs write, then closes
// the indexer to flush the single bulk request the recorder captures. A new
// indexer per write keeps each write's request in its own snapshot.
func captureBulkWrite(t *testing.T, client *esclient.Client, rec *snapshottest.Recorder, store *SamplingStore, write func()) string {
	bulkWriter, err := esclient.NewBulkIndexer(client, esclient.BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	store.batchWriter = bulkWriter
	rec.Reset()
	write()
	require.NoError(t, bulkWriter.Close())
	return rec.Marshal(t)
}
