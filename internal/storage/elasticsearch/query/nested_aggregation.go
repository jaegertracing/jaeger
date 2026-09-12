// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// NestedAggregation is a single-bucket aggregation that targets a nested field
// path and applies its sub-aggregations to the nested documents found under it.
// It renders to {"nested": {"path": path}} with an optional sibling
// "aggregations".
type NestedAggregation struct {
	path    string
	subAggs map[string]Aggregation
}

// NewNestedAggregation creates a NestedAggregation targeted at the given nested
// field path.
func NewNestedAggregation(path string) *NestedAggregation {
	return &NestedAggregation{path: path}
}

// SubAggregation nests agg under this nested aggregation.
func (a *NestedAggregation) SubAggregation(name string, agg Aggregation) *NestedAggregation {
	if a.subAggs == nil {
		a.subAggs = make(map[string]Aggregation)
	}
	a.subAggs[name] = agg
	return a
}

func (a *NestedAggregation) Source() (any, error) {
	result := map[string]any{
		"nested": map[string]any{"path": a.path},
	}
	aggs, err := subAggregationsSource(a.subAggs)
	if err != nil {
		return nil, err
	}
	if aggs != nil {
		result["aggregations"] = aggs
	}
	return result, nil
}
