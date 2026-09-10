// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCumulativeSumAggregationSource reproduces the cumulative_requests pipeline
// aggregation from testdata/get_call_rates.json.
func TestCumulativeSumAggregationSource(t *testing.T) {
	src, err := NewCumulativeSumAggregation().BucketsPath("_count").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"cumulative_sum": map[string]any{"buckets_path": "_count"},
	}, src)
}
