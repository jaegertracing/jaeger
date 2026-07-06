// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
)

// TestSearchRequestVersionGating covers the query parameters Search always
// sets: rest_total_hits_as_int and ignore_unavailable. The full wire format of
// the service/operation searches is frozen end-to-end by the caller-level
// snapshots in the tracestore/core package (get_services/get_operations), which
// drive a real SearchClient — so there's no snapshot here, only the params this
// package alone owns.
func TestSearchRequestVersionGating(t *testing.T) {
	for _, tt := range []struct {
		version     es.BackendVersion
		wantRestInt bool
	}{
		{es.ElasticV7, true},
	} {
		t.Run(tt.version.String(), func(t *testing.T) {
			rec, url := okServer(t)
			sc := SearchClient{Client: makeClient(t, url, "", "", tt.version)}
			_, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{})
			require.NoError(t, err)

			req := rec.Requests()[0]
			assert.Equal(t, "true", req.Query.Get("ignore_unavailable"))
			assert.Equal(t, tt.wantRestInt, req.Query.Has("rest_total_hits_as_int"))
		})
	}
}

// TestSearchEmptyIndicesPath verifies that searching with no indices produces a
// clean "/_search" path rather than a double-slashed "//_search".
func TestSearchEmptyIndicesPath(t *testing.T) {
	rec, url := okServer(t)
	sc := SearchClient{Client: makeClient(t, url, "", "", es.ElasticV7)}
	_, err := sc.Search(context.Background(), nil, SearchRequest{})
	require.NoError(t, err)
	assert.Equal(t, "/_search", rec.Requests()[0].Path)
}

func TestSearchParsesAggregationBuckets(t *testing.T) {
	const body = `{"aggregations":{"distinct_services":{"buckets":[` +
		`{"key":"svc-a","doc_count":3},{"key":"svc-b","doc_count":1}]}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}))
	defer server.Close()

	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	resp, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{
		Query: query.NewTermQuery("serviceName", "svc-a"),
		Aggregations: map[string]query.Aggregation{
			"distinct_services": query.NewTermsAggregation("serviceName").Size(10),
		},
	})
	require.NoError(t, err)
	buckets := resp.Aggregations["distinct_services"].Buckets
	require.Len(t, buckets, 2)
	assert.Equal(t, "svc-a", buckets[0].Key)
	assert.Equal(t, 3, buckets[0].DocCount)
	assert.Equal(t, "svc-b", buckets[1].Key)
}

// errSource is a query/aggregation node whose Source always fails.
type errSource struct{}

func (errSource) Source() (any, error) { return nil, errors.New("source boom") }

func TestSearchQuerySourceError(t *testing.T) {
	sc := SearchClient{Client: makeClient(t, "http://localhost:9200", "", "", es.ElasticV7)}
	_, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{Query: errSource{}})
	require.ErrorContains(t, err, "source boom")
}

func TestSearchAggregationSourceError(t *testing.T) {
	sc := SearchClient{Client: makeClient(t, "http://localhost:9200", "", "", es.ElasticV7)}
	_, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{
		Aggregations: map[string]query.Aggregation{"a": errSource{}},
	})
	require.ErrorContains(t, err, "source boom")
}

func TestSearchTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	_, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{})
	require.Error(t, err)
}

func TestSearchMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	_, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{})
	require.Error(t, err)
}
