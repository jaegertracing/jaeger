// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// TermsAggregation buckets documents by the distinct values of a field. It
// renders to {"terms": {"field": field, ...}} — optionally with a bucket "order"
// and sibling "aggregations" (sub-aggregations).
type TermsAggregation struct {
	field   string
	size    int
	order   []map[string]string
	subAggs map[string]Aggregation
}

// NewTermsAggregation creates a TermsAggregation on the given field.
func NewTermsAggregation(field string) *TermsAggregation {
	return &TermsAggregation{field: field}
}

// Size bounds the number of distinct buckets returned.
func (a *TermsAggregation) Size(size int) *TermsAggregation {
	a.size = size
	return a
}

// Order sorts buckets by the named metric (e.g. a sub-aggregation) in the given
// direction. Repeated calls append tie-breakers, and the whole set always renders
// as an array.
func (a *TermsAggregation) Order(name string, direction SortDirection) *TermsAggregation {
	a.order = append(a.order, map[string]string{name: string(direction)})
	return a
}

// SubAggregation nests agg under this terms aggregation.
func (a *TermsAggregation) SubAggregation(name string, agg Aggregation) *TermsAggregation {
	if a.subAggs == nil {
		a.subAggs = make(map[string]Aggregation)
	}
	a.subAggs[name] = agg
	return a
}

func (a *TermsAggregation) Source() (any, error) {
	terms := map[string]any{"field": a.field}
	if a.size > 0 {
		terms["size"] = a.size
	}
	if len(a.order) > 0 {
		order := make([]any, len(a.order))
		for i, o := range a.order {
			order[i] = o
		}
		terms["order"] = order
	}
	result := map[string]any{"terms": terms}
	aggs, err := subAggregationsSource(a.subAggs)
	if err != nil {
		return nil, err
	}
	if aggs != nil {
		result["aggregations"] = aggs
	}
	return result, nil
}
