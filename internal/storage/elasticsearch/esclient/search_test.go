// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	agg, ok := resp.Aggregations.Terms("distinct_services")
	require.True(t, ok)
	buckets := agg.Buckets
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

func TestSearchRequestBodyOmitsUnsetPaginationFields(t *testing.T) {
	body, err := SearchRequest{Size: 0, Query: query.NewTermQuery("a", 1)}.body()
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	assert.NotContains(t, m, "sort")
	assert.NotContains(t, m, "search_after")
	assert.NotContains(t, m, "track_total_hits")
	assert.NotContains(t, m, "aggregations")
}

func TestSearchRequestBodyIncludesPaginationFields(t *testing.T) {
	body, err := SearchRequest{
		Size:           100,
		Query:          query.NewTermQuery("traceID", "abc"),
		Sort:           []SortOrder{{Field: "startTime", Order: query.Ascending}},
		SearchAfter:    []any{uint64(1577847845000000)},
		TrackTotalHits: true,
	}.body()
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	assert.Equal(t, []any{map[string]any{"startTime": map[string]any{"order": "asc"}}}, m["sort"])
	assert.Equal(t, []any{float64(1577847845000000)}, m["search_after"])
	assert.Equal(t, true, m["track_total_hits"])
}

func TestSearchParsesHits(t *testing.T) {
	const body = `{"hits":{"total":2,"hits":[` +
		`{"_source":{"traceID":"abc"}},{"_source":{"traceID":"def"}}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	resp, err := sc.Search(context.Background(), []string{"idx"}, SearchRequest{})
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Hits.Total.Value)
	require.Len(t, resp.Hits.Hits, 2)
	assert.JSONEq(t, `{"traceID":"abc"}`, string(resp.Hits.Hits[0].Source))
}

func TestMultiSearchNDJSONAndResponse(t *testing.T) {
	const respBody = `{"responses":[{"hits":{"total":1,"hits":[{"_source":{"traceID":"abc"}}]}}]}`
	var gotMethod, gotPath, gotCT string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotCT = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(respBody))
	}))
	defer server.Close()

	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	resps, err := sc.MultiSearch(context.Background(), []MultiSearchRequest{{
		Indices: []string{"jaeger-span-read"},
		Search: SearchRequest{
			Size:           100,
			Query:          query.NewTermQuery("traceID", "abc"),
			Sort:           []SortOrder{{Field: "startTime", Order: query.Ascending}},
			SearchAfter:    []any{uint64(1)},
			TrackTotalHits: true,
		},
	}})
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/_msearch", gotPath)
	assert.Equal(t, "application/x-ndjson", gotCT)
	lines := strings.Split(strings.TrimRight(string(gotBody), "\n"), "\n")
	require.Len(t, lines, 2, "each request is a header line plus a body line")
	assert.JSONEq(t, `{"ignore_unavailable":true,"index":"jaeger-span-read"}`, lines[0])
	assert.Contains(t, lines[1], `"search_after":[1]`)
	assert.Contains(t, lines[1], `"track_total_hits":true`)

	require.Len(t, resps, 1)
	assert.Equal(t, 1, resps[0].Hits.Total.Value)
	require.Len(t, resps[0].Hits.Hits, 1)
}

func TestMultiSearchMultipleIndicesRenderAsArray(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	_, err := sc.MultiSearch(context.Background(), []MultiSearchRequest{{
		Indices: []string{"idx-a", "idx-b"},
		Search:  SearchRequest{Size: 1},
	}})
	require.NoError(t, err)
	header := strings.SplitN(string(gotBody), "\n", 2)[0]
	assert.JSONEq(t, `{"ignore_unavailable":true,"index":["idx-a","idx-b"]}`, header)
}

func TestMultiSearchBodySourceError(t *testing.T) {
	sc := SearchClient{Client: makeClient(t, "http://localhost:9200", "", "", es.ElasticV7)}
	_, err := sc.MultiSearch(context.Background(), []MultiSearchRequest{{
		Indices: []string{"idx"},
		Search:  SearchRequest{Query: errSource{}},
	}})
	require.ErrorContains(t, err, "source boom")
}

func TestMultiSearchMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	_, err := sc.MultiSearch(context.Background(), []MultiSearchRequest{{Indices: []string{"idx"}}})
	require.Error(t, err)
}

func TestMultiSearchEmptyIndicesOmitsIndexHeader(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	_, err := sc.MultiSearch(context.Background(), []MultiSearchRequest{{Search: SearchRequest{Size: 1}}})
	require.NoError(t, err)
	header := strings.SplitN(string(gotBody), "\n", 2)[0]
	assert.JSONEq(t, `{"ignore_unavailable":true}`, header)
}

func TestMultiSearchTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	sc := SearchClient{Client: makeClient(t, server.URL, "", "", es.ElasticV7)}
	_, err := sc.MultiSearch(context.Background(), []MultiSearchRequest{{Indices: []string{"idx"}}})
	require.Error(t, err)
}

func TestTotalHitsUnmarshalBothForms(t *testing.T) {
	var intForm TotalHits
	require.NoError(t, json.Unmarshal([]byte(`5`), &intForm))
	assert.Equal(t, 5, intForm.Value)

	var objForm TotalHits
	require.NoError(t, json.Unmarshal([]byte(`{"value":7,"relation":"eq"}`), &objForm))
	assert.Equal(t, 7, objForm.Value)

	var bad TotalHits
	require.Error(t, json.Unmarshal([]byte(`"nope"`), &bad))
}
