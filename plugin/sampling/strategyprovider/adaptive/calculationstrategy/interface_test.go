// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package calculationstrategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateFunc(t *testing.T) {
	c := CalculateFunc(func(targetQPS, _ /* qps */, _ /* oldProbability */ float64) float64 {
		return targetQPS
	})
	val := 1.0
	assert.InDelta(t, val, c.Calculate(val, 0, 0), 0.01)
}
