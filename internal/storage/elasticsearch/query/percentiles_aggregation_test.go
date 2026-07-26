// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPercentilesAggregationSource reproduces the percentiles_of_bucket sub-agg
// from testdata/get_latencies.json.
func TestPercentilesAggregationSource(t *testing.T) {
	src, err := NewPercentilesAggregation().Field("duration").Percentiles(95).Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"percentiles": map[string]any{
			"field":    "duration",
			"percents": []float64{95},
		},
	}, src)
}

// TestPercentilesAggregationEmpty renders an empty inner object when neither field
// nor percents are set.
func TestPercentilesAggregationEmpty(t *testing.T) {
	src, err := NewPercentilesAggregation().Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"percentiles": map[string]any{}}, src)
}
