// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// CumulativeSumAggregation is a pipeline aggregation that accumulates a metric
// across the buckets of its parent histogram. It renders to {"cumulative_sum":
// {"buckets_path": …}}, matching what the storage layer previously produced. It
// backs the metricstore call rate.
type CumulativeSumAggregation struct {
	bucketsPath string
}

// NewCumulativeSumAggregation creates an empty CumulativeSumAggregation.
func NewCumulativeSumAggregation() *CumulativeSumAggregation {
	return &CumulativeSumAggregation{}
}

// BucketsPath sets the path to the metric to accumulate (e.g. "_count").
func (a *CumulativeSumAggregation) BucketsPath(path string) *CumulativeSumAggregation {
	a.bucketsPath = path
	return a
}

func (a *CumulativeSumAggregation) Source() (any, error) {
	return map[string]any{
		"cumulative_sum": map[string]any{"buckets_path": a.bucketsPath},
	}, nil
}
