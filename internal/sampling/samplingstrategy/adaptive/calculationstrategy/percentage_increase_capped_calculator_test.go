// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package calculationstrategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPercentageIncreaseCappedCalculator(t *testing.T) {
	calculator := NewPercentageIncreaseCappedCalculator(0)
	tests := []struct {
		targetQPS           float64
		curQPS              float64
		oldProbability      float64
		expectedProbability float64
		testName            string
	}{
		{1.0, 2.0, 0.1, 0.05, "test1"},
		{1.0, 0.5, 0.1, 0.15, "test2"},
		{1.0, 0.8, 0.1, 0.125, "test3"},
	}
	for _, tt := range tests {
		probability := calculator.Calculate(tt.targetQPS, tt.curQPS, tt.oldProbability)
		assert.InDelta(t, tt.expectedProbability, probability, 1e-4, tt.testName)
	}
}
