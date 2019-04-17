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
	"math"
	"sync"
)

// weightVectorCache stores normalizing weights of different lengths. The head of the weights slice
// contains the largest weight.
type weightVectorCache struct {
	sync.Mutex

	cache map[int][]float64
}

// newweightVectorCache returns a new weights vector cache.
func newWeightVectorCache() *weightVectorCache {
	// TODO allow users to plugin different weighting algorithms
	return &weightVectorCache{
		cache: make(map[int][]float64),
	}
}

// getWeights returns weights for the specified length { w(i) = i ^ 4, i=1..L }, normalized.
func (c *weightVectorCache) getWeights(length int) []float64 {
	c.Lock()
	defer c.Unlock()
	if weights, ok := c.cache[length]; ok {
		return weights
	}
	weights := make([]float64, 0, length)
	var sum float64
	for i := length; i > 0; i-- {
		w := math.Pow(float64(i), 4)
		weights = append(weights, w)
		sum += w
	}
	// normalize
	for i := 0; i < length; i++ {
		weights[i] = weights[i] / sum
	}
	c.cache[length] = weights
	return weights
}
