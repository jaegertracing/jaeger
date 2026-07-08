// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// TopHitsAggregation returns the top matching documents within a bucket. It
// renders to {"top_hits": {...}} with optional size, sort, and _source includes.
type TopHitsAggregation struct {
	size           int
	sorts          []map[string]any
	sourceIncludes []string
}

// NewTopHitsAggregation creates an empty TopHitsAggregation.
func NewTopHitsAggregation() *TopHitsAggregation {
	return &TopHitsAggregation{}
}

// Size bounds the number of hits returned per bucket.
func (a *TopHitsAggregation) Size(size int) *TopHitsAggregation {
	a.size = size
	return a
}

// Sort orders the hits by field in the given direction.
func (a *TopHitsAggregation) Sort(field string, order SortDirection) *TopHitsAggregation {
	a.sorts = append(a.sorts, map[string]any{field: map[string]any{"order": string(order)}})
	return a
}

// SourceIncludes restricts the returned _source to the named fields.
func (a *TopHitsAggregation) SourceIncludes(fields ...string) *TopHitsAggregation {
	a.sourceIncludes = append(a.sourceIncludes, fields...)
	return a
}

func (a *TopHitsAggregation) Source() (any, error) {
	topHits := map[string]any{}
	if a.size > 0 {
		topHits["size"] = a.size
	}
	if len(a.sorts) > 0 {
		sorts := make([]any, len(a.sorts))
		for i, s := range a.sorts {
			sorts[i] = s
		}
		topHits["sort"] = sorts
	}
	if len(a.sourceIncludes) > 0 {
		topHits["_source"] = map[string]any{"includes": a.sourceIncludes}
	}
	return map[string]any{"top_hits": topHits}, nil
}
