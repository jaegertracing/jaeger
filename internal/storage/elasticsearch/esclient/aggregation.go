// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/json"
	"fmt"
)

// subAggregations holds the named sub-aggregation results of a bucket or filter,
// captured raw and decoded on demand by the typed accessors below. Nesting
// (filter → top_hits, terms → filter, …) is navigated by chaining them.
type subAggregations map[string]json.RawMessage

// Metric returns the value of a single-value metric sub-aggregation (min/max).
// The value is a pointer so a null metric (no matching docs) is distinguishable
// from zero.
func (a subAggregations) Metric(name string) (*float64, bool) {
	raw, ok := a[name]
	if !ok {
		return nil, false
	}
	var m struct {
		Value *float64 `json:"value"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	return m.Value, true
}

// Filter returns a filter sub-aggregation's result (its doc_count and any of its
// own sub-aggregations).
func (a subAggregations) Filter(name string) (*FilterResult, bool) {
	raw, ok := a[name]
	if !ok {
		return nil, false
	}
	var f FilterResult
	if json.Unmarshal(raw, &f) != nil {
		return nil, false
	}
	return &f, true
}

// Terms returns a terms sub-aggregation's buckets.
func (a subAggregations) Terms(name string) (*AggregationResult, bool) {
	raw, ok := a[name]
	if !ok {
		return nil, false
	}
	var r AggregationResult
	if json.Unmarshal(raw, &r) != nil {
		return nil, false
	}
	return &r, true
}

// TopHits returns the hits of a top_hits sub-aggregation.
func (a subAggregations) TopHits(name string) (*HitsResult, bool) {
	raw, ok := a[name]
	if !ok {
		return nil, false
	}
	var h struct {
		Hits HitsResult `json:"hits"`
	}
	if json.Unmarshal(raw, &h) != nil {
		return nil, false
	}
	return &h.Hits, true
}

// AggregationBucket is a single bucket: its key, document count, and any nested
// sub-aggregations (reached through the promoted subAggregations accessors).
type AggregationBucket struct {
	Key      string
	DocCount int
	subAggregations
}

func (b *AggregationBucket) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// Bucket keys are strings for the fields we aggregate (traceID, serviceName).
	// A present-but-non-string key means a mapping regression, so fail the decode
	// rather than silently yield an empty key that callers treat as a valid ID.
	if k, ok := raw["key"]; ok {
		if err := json.Unmarshal(k, &b.Key); err != nil {
			return fmt.Errorf("aggregation bucket has a non-string key: %w", err)
		}
		delete(raw, "key")
	}
	if dc, ok := raw["doc_count"]; ok {
		if err := json.Unmarshal(dc, &b.DocCount); err != nil {
			return err
		}
		delete(raw, "doc_count")
	}
	b.subAggregations = raw
	return nil
}

// FilterResult is a filter aggregation's result: the matched document count and
// any of its own sub-aggregations.
type FilterResult struct {
	DocCount int
	subAggregations
}

func (f *FilterResult) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if dc, ok := raw["doc_count"]; ok {
		if err := json.Unmarshal(dc, &f.DocCount); err != nil {
			return err
		}
		delete(raw, "doc_count")
	}
	f.subAggregations = raw
	return nil
}
