// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		assert.InDelta(t, probability, tt.expectedProbability, 0.0001, tt.testName)
	}
}
