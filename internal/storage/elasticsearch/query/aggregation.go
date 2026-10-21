// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// subAggregationsSource renders named sub-aggregations into the map that becomes
// the "aggregations" field of an enclosing aggregation, or nil when there are
// none (so the key is omitted).
func subAggregationsSource(subAggs map[string]Aggregation) (map[string]any, error) {
	if len(subAggs) == 0 {
		return nil, nil
	}
	aggs := make(map[string]any, len(subAggs))
	for name, agg := range subAggs {
		src, err := agg.Source()
		if err != nil {
			return nil, err
		}
		aggs[name] = src
	}
	return aggs, nil
}
