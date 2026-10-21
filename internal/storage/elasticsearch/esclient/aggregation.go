// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/json"
	"fmt"
)

// Aggregations holds named aggregation results — both the top level of a response
// and the sub-aggregations of a bucket or filter — captured raw and decoded on
// demand by the typed accessors below. Nesting (terms → date_histogram →
// percentiles, filter → top_hits, …) is navigated by chaining them.
type Aggregations map[string]json.RawMessage

// Metric returns the value of a single-value metric aggregation (min/max/
// cumulative_sum). The value is a pointer so a null metric (no matching docs) is
// distinguishable from zero.
func (a Aggregations) Metric(name string) (*float64, bool) {
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

// Filter returns a filter aggregation's result (its doc_count and any of its own
// sub-aggregations).
func (a Aggregations) Filter(name string) (*FilterResult, bool) {
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

// Terms returns a terms aggregation's (string-keyed) buckets.
func (a Aggregations) Terms(name string) (*AggregationResult, bool) {
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

// DateHistogram returns a date_histogram aggregation's (time-keyed) buckets.
func (a Aggregations) DateHistogram(name string) (*HistogramResult, bool) {
	raw, ok := a[name]
	if !ok {
		return nil, false
	}
	var r HistogramResult
	if json.Unmarshal(raw, &r) != nil {
		return nil, false
	}
	return &r, true
}

// Percentiles returns a percentiles aggregation's values, keyed by percentile
// rank as Elasticsearch formats it (e.g. "95.0").
func (a Aggregations) Percentiles(name string) (*PercentilesResult, bool) {
	raw, ok := a[name]
	if !ok {
		return nil, false
	}
	var p PercentilesResult
	if json.Unmarshal(raw, &p) != nil {
		return nil, false
	}
	return &p, true
}

// TopHits returns the hits of a top_hits aggregation.
func (a Aggregations) TopHits(name string) (*HitsResult, bool) {
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

// AggregationBucket is a single terms bucket: its (string) key, document count,
// and any nested sub-aggregations (reached through the promoted Aggregations
// accessors).
type AggregationBucket struct {
	Key      string
	DocCount int
	Aggregations
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
	b.Aggregations = raw
	return nil
}

func (b AggregationBucket) MarshalJSON() ([]byte, error) {
	return marshalBucket(b.Key, b.DocCount, b.Aggregations)
}

// HistogramResult holds the buckets of a date_histogram aggregation.
type HistogramResult struct {
	Buckets []HistogramBucket `json:"buckets"`
}

// HistogramBucket is a single date_histogram bucket: its key (epoch millis),
// document count, and any nested sub-aggregations. Unlike a terms bucket its key
// is numeric, so it is a distinct type rather than reusing AggregationBucket
// (whose key is strictly a string).
type HistogramBucket struct {
	Key      int64
	DocCount int
	Aggregations
}

func (b *HistogramBucket) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if k, ok := raw["key"]; ok {
		if err := json.Unmarshal(k, &b.Key); err != nil {
			return err
		}
		delete(raw, "key")
	}
	// key_as_string mirrors the numeric key; it is not needed once key is parsed.
	delete(raw, "key_as_string")
	if dc, ok := raw["doc_count"]; ok {
		if err := json.Unmarshal(dc, &b.DocCount); err != nil {
			return err
		}
		delete(raw, "doc_count")
	}
	b.Aggregations = raw
	return nil
}

func (b HistogramBucket) MarshalJSON() ([]byte, error) {
	return marshalBucket(b.Key, b.DocCount, b.Aggregations)
}

// PercentilesResult holds a percentiles aggregation's computed values, keyed by
// percentile rank as Elasticsearch formats it (e.g. "95.0"). A percentile value is
// null only when the aggregation matched no documents; a JSON null decodes into a
// float64 as a no-op (0), not an error, so the map stays present and the accessor
// still returns true. A plain float64 (rather than *float64) is enough because the
// only reader skips empty buckets — bucketsToPoints short-circuits doc_count==0 to
// NaN before any percentile is read — so a present-but-null value never surfaces.
type PercentilesResult struct {
	Values map[string]float64 `json:"values"`
}

// FilterResult is a filter aggregation's result: the matched document count and
// any of its own sub-aggregations.
type FilterResult struct {
	DocCount int
	Aggregations
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
	f.Aggregations = raw
	return nil
}

// marshalBucket renders a bucket back to its wire shape (key + doc_count merged
// with the raw sub-aggregations). It exists so tests can construct responses from
// typed values; production only ever decodes buckets. The sub-aggregations are
// json.RawMessage values; json.Marshal validates each one (the encoder compacts a
// Marshaler's output), so a malformed raw sub-aggregation surfaces as a marshal
// error rather than being emitted verbatim.
func marshalBucket(key any, docCount int, subAggs Aggregations) ([]byte, error) {
	m := make(map[string]any, len(subAggs)+2)
	for k, v := range subAggs {
		m[k] = v
	}
	m["key"] = key
	m["doc_count"] = docCount
	return json.Marshal(m)
}
