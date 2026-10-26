// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDateHistogramAggregationSource reproduces the results_buckets date_histogram
// from testdata/get_call_rates.json: a fixed-interval histogram over startTimeMillis
// with extended bounds and a sub-aggregation.
func TestDateHistogramAggregationSource(t *testing.T) {
	agg := NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval("60000ms").
		MinDocCount(0).
		ExtendedBounds(1577930045000, 1577934245000).
		SubAggregation("cumulative_requests", NewCumulativeSumAggregation().BucketsPath("_count"))
	src, err := agg.Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"date_histogram": map[string]any{
			"field":          "startTimeMillis",
			"fixed_interval": "60000ms",
			"min_doc_count":  0,
			"extended_bounds": map[string]any{
				"min": int64(1577930045000),
				"max": int64(1577934245000),
			},
		},
		"aggregations": map[string]any{
			"cumulative_requests": map[string]any{
				"cumulative_sum": map[string]any{"buckets_path": "_count"},
			},
		},
	}, src)
}

// TestDateHistogramAggregationMinimal omits the optional knobs, so only the fields
// that were set appear (no min_doc_count, no extended_bounds, no aggregations).
func TestDateHistogramAggregationMinimal(t *testing.T) {
	src, err := NewDateHistogramAggregation().Field("startTimeMillis").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"date_histogram": map[string]any{"field": "startTimeMillis"},
	}, src)
}

func TestDateHistogramAggregationSubAggregationPropagatesError(t *testing.T) {
	_, err := NewDateHistogramAggregation().
		Field("startTimeMillis").
		SubAggregation("bad", errAgg{}).
		Source()
	require.ErrorIs(t, err, errBadQuery)
}
