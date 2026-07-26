// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
)

// SearchRequest is a driver-neutral search body: an optional query, named
// aggregations, the number of documents to return, and the sort / search_after /
// track_total_hits controls the paginated trace read needs. The storage layer
// builds it from the owned query AST, so no driver type crosses this boundary.
type SearchRequest struct {
	Query          query.Query
	Aggregations   map[string]query.Aggregation
	Size           int
	Sort           []SortOrder
	SearchAfter    []any
	TrackTotalHits bool
}

// SortOrder sorts hits by a field. It renders to {field: {"order": order}} where
// order is query.Ascending or query.Descending.
type SortOrder struct {
	Field string
	Order query.SortDirection
}

func (r SearchRequest) body() ([]byte, error) {
	m := map[string]any{"size": r.Size}
	if r.Query != nil {
		q, err := r.Query.Source()
		if err != nil {
			return nil, err
		}
		m["query"] = q
	}
	if len(r.Aggregations) > 0 {
		aggs := make(map[string]any, len(r.Aggregations))
		for name, agg := range r.Aggregations {
			src, err := agg.Source()
			if err != nil {
				return nil, err
			}
			aggs[name] = src
		}
		m["aggregations"] = aggs
	}
	if len(r.Sort) > 0 {
		sort := make([]any, len(r.Sort))
		for i, s := range r.Sort {
			sort[i] = map[string]any{s.Field: map[string]any{"order": s.Order}}
		}
		m["sort"] = sort
	}
	if len(r.SearchAfter) > 0 {
		m["search_after"] = r.SearchAfter
	}
	if r.TrackTotalHits {
		m["track_total_hits"] = true
	}
	return json.Marshal(m)
}

// SearchResponse is the owned, driver-neutral search response. It exposes the
// matched documents (hits) and aggregation buckets; other aggregation shapes are
// added by later migration slices as their callers need them.
type SearchResponse struct {
	// Error and Status report a failed _msearch item; see Err. A failed item
	// carries an error object and a non-2xx status instead of hits, inside an
	// overall HTTP 200 _msearch response.
	Error  json.RawMessage `json:"error,omitempty"`
	Status int             `json:"status,omitempty"`

	Hits         HitsResult   `json:"hits"`
	Aggregations Aggregations `json:"aggregations"`
}

// Err returns the response's server-reported failure, or nil if it succeeded.
// Only a MultiSearch item can fail this way: _msearch reports a failed item as
// {"error": ..., "status": N} with no hits inside an overall HTTP 200, so
// without this check a failed item is indistinguishable from empty hits. (A
// failed single Search surfaces as a transport-level error instead.)
func (r *SearchResponse) Err() error {
	if len(r.Error) == 0 || bytes.Equal(r.Error, []byte("null")) {
		return nil
	}
	return fmt.Errorf("search failed with status %d: %s", r.Status, r.Error)
}

// HitsResult holds the documents a search matched and, when the request asked for
// it (track_total_hits), the total number of matches — used to page the trace read.
type HitsResult struct {
	Total TotalHits   `json:"total"`
	Hits  []SearchHit `json:"hits"`
}

// SearchHit is a single matched document. Source is the raw _source JSON, left
// unparsed so the storage layer unmarshals it into its own dbmodel type — the
// client never knows what a span or throughput document is.
type SearchHit struct {
	Source json.RawMessage `json:"_source"`
}

// TotalHits is the number of matching documents. Elasticsearch reports it either
// as a plain integer (with rest_total_hits_as_int) or as an object {"value": n,
// "relation": ...}; TotalHits accepts both so callers see a single int.
type TotalHits struct {
	Value int
}

func (t *TotalHits) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		t.Value = n
		return nil
	}
	var obj struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	t.Value = obj.Value
	return nil
}

// AggregationResult holds the buckets of a bucketing aggregation (e.g. terms).
type AggregationResult struct {
	Buckets []AggregationBucket `json:"buckets"`
}

// SearchClient is the data-plane search API over the shared transport, analogous
// to IndicesClient/ILMClient on the admin plane.
type SearchClient struct {
	*Client
}

var _ Searcher = SearchClient{}

// Search issues req against the given indices and returns the owned response.
func (s SearchClient) Search(ctx context.Context, indices []string, req SearchRequest) (*SearchResponse, error) {
	body, err := req.body()
	if err != nil {
		return nil, err
	}
	// ignore_unavailable is always set; rest_total_hits_as_int makes ES7+/OS
	// report total hits as a plain number. With no indices the endpoint stays
	// relative ("_search"), so request's "/" prefix doesn't produce a double slash.
	endpoint := "_search?ignore_unavailable=true&rest_total_hits_as_int=true"
	if len(indices) > 0 {
		endpoint = strings.Join(indices, ",") + "/" + endpoint
	}
	raw, err := s.request(ctx, elasticRequest{
		endpoint: endpoint,
		method:   http.MethodPost,
		body:     body,
	})
	if err != nil {
		return nil, err
	}
	var resp SearchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// MultiSearchRequest is one sub-request of a _msearch: a search body and the
// indices it targets.
type MultiSearchRequest struct {
	Indices []string
	Search  SearchRequest
}

// MultiSearch issues all reqs in a single _msearch and returns one response per
// request, in order. Each sub-request contributes an NDJSON header line (its
// indices and ignore_unavailable) followed by its search body. A single index
// renders as a string and several as an array, matching what the storage layer
// previously produced.
func (s SearchClient) MultiSearch(ctx context.Context, reqs []MultiSearchRequest) ([]SearchResponse, error) {
	var buf bytes.Buffer
	for _, r := range reqs {
		header := map[string]any{"ignore_unavailable": true}
		switch len(r.Indices) {
		case 0:
		case 1:
			header["index"] = r.Indices[0]
		default:
			header["index"] = r.Indices
		}
		headerJSON, err := json.Marshal(header)
		if err != nil {
			return nil, err
		}
		bodyJSON, err := r.Search.body()
		if err != nil {
			return nil, err
		}
		buf.Write(headerJSON)
		buf.WriteByte('\n')
		buf.Write(bodyJSON)
		buf.WriteByte('\n')
	}
	raw, err := s.request(ctx, elasticRequest{
		endpoint:    "_msearch",
		method:      http.MethodPost,
		body:        buf.Bytes(),
		contentType: "application/x-ndjson",
	})
	if err != nil {
		return nil, err
	}
	// A per-sub-response error (an item carrying an "error"/non-2xx "status" while
	// the overall _msearch is HTTP 200) is decoded into the item's Error/Status
	// fields; callers must check Err() per item, because a failed item carries no
	// hits and would otherwise be indistinguishable from an empty result.
	var resp struct {
		Responses []SearchResponse `json:"responses"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Responses, nil
}
