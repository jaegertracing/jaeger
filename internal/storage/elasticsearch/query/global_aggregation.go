// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// GlobalAggregation is a single-bucket aggregation that ignores the query
// context and executes its sub-aggregations across all documents. It renders
// to {"global": {}} with an optional sibling "aggregations".
type GlobalAggregation struct {
	subAggs map[string]Aggregation
}

// NewGlobalAggregation creates a GlobalAggregation.
func NewGlobalAggregation() *GlobalAggregation {
	return &GlobalAggregation{}
}

// SubAggregation nests agg under this global aggregation.
func (a *GlobalAggregation) SubAggregation(name string, agg Aggregation) *GlobalAggregation {
	if a.subAggs == nil {
		a.subAggs = make(map[string]Aggregation)
	}
	a.subAggs[name] = agg
	return a
}

func (a *GlobalAggregation) Source() (any, error) {
	result := map[string]any{"global": struct{}{}}
	aggs, err := subAggregationsSource(a.subAggs)
	if err != nil {
		return nil, err
	}
	if aggs != nil {
		result["aggregations"] = aggs
	}
	return result, nil
}
