// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWeights(t *testing.T) {
	c := NewWeightVectorCache()

	weights := c.GetWeights(1)
	assert.Len(t, weights, 1)

	weights = c.GetWeights(3)
	assert.Len(t, weights, 3)
	assert.InDelta(t, 0.8265306122448979, weights[0], 0.001)

	weights = c.GetWeights(5)
	assert.Len(t, weights, 5)
	assert.InDelta(t, 0.6384, weights[0], 0.001)
	assert.InDelta(t, 0.0010, weights[4], 0.001)
}
