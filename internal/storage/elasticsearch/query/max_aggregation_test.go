// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxAggregationSource(t *testing.T) {
	src, err := NewMaxAggregation("startTime").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"max": map[string]any{"field": "startTime"}}, src)
}

// TestTermsAggregationWithOrderAndSubAgg reproduces the traceIDs aggregation from
// testdata/find_trace_ids.json: a terms agg ordered by a startTime max sub-agg.
func TestTermsAggregationWithOrderAndSubAgg(t *testing.T) {
	agg := NewTermsAggregation("traceID").
		Size(20).
		Order("startTime", "desc").
		SubAggregation("startTime", NewMaxAggregation("startTime"))
	src, err := agg.Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"terms": map[string]any{
			"field": "traceID",
			"size":  20,
			"order": []any{map[string]string{"startTime": "desc"}},
		},
		"aggregations": map[string]any{
			"startTime": map[string]any{"max": map[string]any{"field": "startTime"}},
		},
	}, src)
}

func TestTermsAggregationSubAggregationPropagatesError(t *testing.T) {
	_, err := NewTermsAggregation("traceID").SubAggregation("bad", errAgg{}).Source()
	require.ErrorIs(t, err, errBadQuery)
}
