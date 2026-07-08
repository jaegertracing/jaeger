// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// FilterAggregation narrows documents to those matching a query, then applies its
// sub-aggregations (or just counts them). It renders to {"filter": <query>} with
// an optional sibling "aggregations".
type FilterAggregation struct {
	filter  Query
	subAggs map[string]Aggregation
}

// NewFilterAggregation creates a FilterAggregation over the given filter query.
func NewFilterAggregation(filter Query) *FilterAggregation {
	return &FilterAggregation{filter: filter}
}

// SubAggregation nests agg under this filter.
func (a *FilterAggregation) SubAggregation(name string, agg Aggregation) *FilterAggregation {
	if a.subAggs == nil {
		a.subAggs = make(map[string]Aggregation)
	}
	a.subAggs[name] = agg
	return a
}

func (a *FilterAggregation) Source() (any, error) {
	filter, err := a.filter.Source()
	if err != nil {
		return nil, err
	}
	result := map[string]any{"filter": filter}
	aggs, err := subAggregationsSource(a.subAggs)
	if err != nil {
		return nil, err
	}
	if aggs != nil {
		result["aggregations"] = aggs
	}
	return result, nil
}
