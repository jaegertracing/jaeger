// Copyright (c) 2025 The Jaeger Authors.
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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
)

// TestSearchRequestSnapshot freezes the wire format of the service/operation
// searches: POST /<index>/_search with a terms aggregation (+ a term query for
// operations). ES7+/OS add rest_total_hits_as_int; ES6 omits it.
func TestSearchRequestSnapshot(t *testing.T) {
	tests := []struct {
		name string
		req  SearchRequest
	}{
		{
			name: "search_services",
			req: SearchRequest{
				Size: 0,
				Aggregations: map[string]query.Aggregation{
					"distinct_services": query.NewTermsAggregation("serviceName").Size(10),
				},
			},
		},
		{
			name: "search_operations",
			req: SearchRequest{
				Size:  0,
				Query: query.NewTermQuery("serviceName", "test-service"),
				Aggregations: map[string]query.Aggregation{
					"distinct_operations": query.NewTermsAggregation("operationName").Size(10),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := map[es.BackendVersion]string{}
			for _, version := range es.AllVersions {
				rec, url := okServer(t)
				sc := SearchClient{Client: makeClient(t, url, "", "", version)}
				_, err := sc.Search(context.Background(), []string{"test-index"}, tt.req)
				require.NoError(t, err)
				content[version] = rec.Marshal(t)
			}
			snapshottest.AssertByVersion(t, "testdata/"+tt.name, content)
		})
	}
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
