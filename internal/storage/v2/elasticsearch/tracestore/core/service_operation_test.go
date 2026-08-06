// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func TestToUpsertItem(t *testing.T) {
	indexName := "jaeger-1995-04-21"
	serviceHash := "de3b5a8f1a79989d"
	jsonSpan := &dbmodel.Span{
		OperationName: "operation",
		Process:       dbmodel.Process{ServiceName: "service"},
	}
	s := NewServiceOperationStorage(nil, zap.NewNop(), time.Hour)

	item, key, ok := s.toUpsertItem(indexName, jsonSpan)
	require.True(t, ok)
	assert.Equal(t, indexName, item.Index)
	assert.Equal(t, serviceHash, item.ID)
	assert.Equal(t, key, item.ID)
	assert.IsType(t, dbmodel.Service{}, item.Body)

	// Still pending on a repeat: the pair is not cached until the write is confirmed
	// durable (RFC 0007 §4.3), so a failed batch would re-send it.
	_, _, ok = s.toUpsertItem(indexName, jsonSpan)
	require.True(t, ok, "must stay pending until confirmed")

	// After confirming, the pair is deduplicated.
	s.commitToCache(key)
	_, _, ok = s.toUpsertItem(indexName, jsonSpan)
	assert.False(t, ok, "must be cached after confirm")
}

// oneBucketResponse is a search response with a single terms-aggregation bucket
// keyed "123", used to assert getServices/getOperations extract the bucket keys.
func oneBucketResponse(aggName string) *esclient.SearchResponse {
	return &esclient.SearchResponse{
		Aggregations: termsAggregations(map[string]esclient.AggregationResult{
			aggName: {Buckets: []esclient.AggregationBucket{{Key: "123", DocCount: 16}}},
		}),
	}
}

// termsAggregations builds the raw-message Aggregations a response carries from
// typed terms results, so tests can assert bucket parsing without hand-writing the
// wire JSON. Marshalling static test data cannot fail.
func termsAggregations(byName map[string]esclient.AggregationResult) esclient.Aggregations {
	aggs := esclient.Aggregations{}
	for name, result := range byName {
		raw, err := json.Marshal(result)
		if err != nil {
			panic(err)
		}
		aggs[name] = raw
	}
	return aggs
}

func TestSpanReader_GetServices(t *testing.T) {
	tests := []struct {
		name     string
		resp     *esclient.SearchResponse
		respErr  error
		expected []string
		errMsg   string
	}{
		{
			name:     "full behavior",
			resp:     oneBucketResponse(servicesAggregation),
			expected: []string{"123"},
		},
		{
			name:   "missing aggregation",
			resp:   &esclient.SearchResponse{Aggregations: termsAggregations(map[string]esclient.AggregationResult{"other": {}})},
			errMsg: "could not find aggregation of " + servicesAggregation,
		},
		{
			name:    "search error",
			respErr: errors.New("Search failure"),
			errMsg:  "search services failed: Search failure",
		},
		{
			name:   "nil response",
			errMsg: "nil search response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				r.searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).Return(tc.resp, tc.respErr)
				services, err := r.reader.GetServices(context.Background())
				if tc.errMsg != "" {
					require.EqualError(t, err, tc.errMsg)
					assert.Nil(t, services)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tc.expected, services)
				}
			})
		})
	}
}

func TestSpanReader_GetOperations(t *testing.T) {
	tests := []struct {
		name     string
		resp     *esclient.SearchResponse
		respErr  error
		expected []dbmodel.Operation
		errMsg   string
	}{
		{
			name:     "full behavior",
			resp:     oneBucketResponse(operationsAggregation),
			expected: []dbmodel.Operation{{Name: "123"}},
		},
		{
			name:   "missing aggregation",
			resp:   &esclient.SearchResponse{Aggregations: termsAggregations(map[string]esclient.AggregationResult{"other": {}})},
			errMsg: "could not find aggregation of " + operationsAggregation,
		},
		{
			name:    "search error",
			respErr: errors.New("Search failure"),
			errMsg:  "search operations failed: Search failure",
		},
		{
			name:   "nil response",
			errMsg: "nil search response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				r.searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).Return(tc.resp, tc.respErr)
				operations, err := r.reader.GetOperations(
					context.Background(),
					dbmodel.OperationQueryParameters{ServiceName: "someService"},
				)
				if tc.errMsg != "" {
					require.EqualError(t, err, tc.errMsg)
					assert.Nil(t, operations)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tc.expected, operations)
				}
			})
		})
	}
}

// TestServiceOperationStorage_ReadWithoutSearcher verifies the read methods
// fail clearly (not with a nil-pointer panic) when the storage was built for
// write-only use, i.e. with a nil searcher.
func TestServiceOperationStorage_ReadWithoutSearcher(t *testing.T) {
	s := NewServiceOperationStorage(nil, zap.NewNop(), 0)

	_, err := s.getServices(context.Background(), []string{"idx"}, 10)
	require.ErrorIs(t, err, errNoSearcher)

	_, err = s.getOperations(context.Background(), []string{"idx"}, "svc", 10)
	require.ErrorIs(t, err, errNoSearcher)
}

func TestSpanReader_GetServicesEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		r.searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).
			Return(&esclient.SearchResponse{}, nil)
		services, err := r.reader.GetServices(context.Background())
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestSpanReader_GetOperationsEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		r.searcher.On("Search", mock.Anything, mock.Anything, mock.Anything).
			Return(&esclient.SearchResponse{}, nil)
		services, err := r.reader.GetOperations(
			context.Background(),
			dbmodel.OperationQueryParameters{ServiceName: "foo"},
		)
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

// TestServiceOperationRequestSnapshots freezes the exact wire format of the
// service/operation read + write path over esclient (SearchClient for reads,
// the bulk indexer for writes). Every supported version emits the same request,
// so the snapshots collapse to a single file per operation.

// dataRecorder answers each request with an empty-but-valid response for its
// endpoint (search, msearch, or bulk), so operations complete without error
// while the request is captured.
func dataRecorder() *snapshottest.Recorder {
	return snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "_bulk"):
			w.Write([]byte(`{"took":0,"errors":false,"items":[]}`))
		case strings.HasSuffix(r.URL.Path, "_msearch"):
			w.Write([]byte(`{"responses":[{"took":0,"hits":{"total":0,"hits":[]}}]}`))
		default:
			w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]}}`))
		}
	})
}

func TestServiceOperationRequestSnapshots(t *testing.T) {
	const (
		readIndex  = "test-jaeger-service-read"
		writeIndex = "test-jaeger-service-write-000001"
	)
	span := &dbmodel.Span{
		OperationName: "test-operation",
		Process:       dbmodel.Process{ServiceName: "test-service"},
	}

	getServices := map[es.BackendVersion]string{}
	getOperations := map[es.BackendVersion]string{}
	writeService := map[es.BackendVersion]string{}

	for _, version := range es.AllVersions {
		rec := dataRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)

		// One real esclient over the recording server backs both the search path
		// and the bulk writer. Version is pinned so the client skips its probe. The
		// documents buffer until Close, which flushes the one bulk request we record.
		esCfg := &config.Configuration{Servers: []string{server.URL}, Version: uint(version)}
		esClient, err := esclient.NewClient(context.Background(), esCfg, zap.NewNop(), nil)
		require.NoError(t, err)
		searcher := esclient.SearchClient{Client: esClient}
		bulkWriter, err := esclient.NewBulkIndexer(esClient, esclient.BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)
		sos := NewServiceOperationStorage(searcher, zap.NewNop(), 0)
		ctx := context.Background()

		rec.Reset()
		_, err = sos.getServices(ctx, []string{readIndex}, 10)
		require.NoError(t, err)
		getServices[version] = rec.Marshal(t)

		rec.Reset()
		_, err = sos.getOperations(ctx, []string{readIndex}, "test-service", 10)
		require.NoError(t, err)
		getOperations[version] = rec.Marshal(t)

		rec.Reset()
		item, _, ok := sos.toUpsertItem(writeIndex, span)
		require.True(t, ok)
		bulkWriter.Add(item)
		require.NoError(t, bulkWriter.Close()) // flushes the bulk request
		writeService[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/get_services", getServices)
	snapshottest.AssertByVersion(t, "testdata/get_operations", getOperations)
	snapshottest.AssertByVersion(t, "testdata/write_service", writeService)
}
