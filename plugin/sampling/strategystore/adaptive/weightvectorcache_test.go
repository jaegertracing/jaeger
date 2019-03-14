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

package adaptive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWeights(t *testing.T) {
	c := newWeightVectorCache()

	weights := c.getWeights(1)
	assert.Len(t, weights, 1)

	weights = c.getWeights(3)
	assert.Len(t, weights, 3)
	assert.InDelta(t, 0.8265306122448979, weights[0], 0.001)

	weights = c.getWeights(5)
	assert.Len(t, weights, 5)
	assert.InDelta(t, 0.6384, weights[0], 0.001)
	assert.InDelta(t, 0.0010, weights[4], 0.001)
}
