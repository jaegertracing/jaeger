// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
)

// SearchRequest is a driver-neutral search body: an optional query, named
// aggregations, and the number of documents to return. The storage layer builds
// it from the owned query AST, so no driver type crosses this boundary.
type SearchRequest struct {
	Query        query.Query
	Aggregations map[string]query.Aggregation
	Size         int
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
	return json.Marshal(m)
}

// SearchResponse is the owned, driver-neutral search response. It exposes the
// matched documents (hits) and aggregation buckets; other aggregation shapes are
// added by later migration slices as their callers need them.
type SearchResponse struct {
	Hits         HitsResult                   `json:"hits"`
	Aggregations map[string]AggregationResult `json:"aggregations"`
}

// HitsResult holds the documents a search matched.
type HitsResult struct {
	Hits []SearchHit `json:"hits"`
}

// SearchHit is a single matched document. Source is the raw _source JSON, left
// unparsed so the storage layer unmarshals it into its own dbmodel type — the
// client never knows what a span or throughput document is.
type SearchHit struct {
	Source json.RawMessage `json:"_source"`
}

// AggregationResult holds the buckets of a bucketing aggregation (e.g. terms).
type AggregationResult struct {
	Buckets []AggregationBucket `json:"buckets"`
}

// AggregationBucket is a single bucket: its key and document count.
type AggregationBucket struct {
	Key      string `json:"key"`
	DocCount int    `json:"doc_count"`
}

// SearchClient is the data-plane search API over the shared transport, analogous
// to IndicesClient/ILMClient on the admin plane.
type SearchClient struct {
	Client
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
