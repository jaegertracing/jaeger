// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"math"
	"sync"
)

// WeightVectorCache stores normalizing weight vectors of different lengths.
// The head of each weight vector contains the largest weight.
type WeightVectorCache struct {
	mu    sync.Mutex
	cache map[int][]float64
}

// NewWeightVectorCache returns a new weights vector cache.
func NewWeightVectorCache() *WeightVectorCache {
	// TODO allow users to plugin different weighting algorithms
	return &WeightVectorCache{
		cache: make(map[int][]float64),
	}
}

// GetWeights returns weights for the specified length { w(i) = i ^ 4, i=1..L }, normalized.
func (c *WeightVectorCache) GetWeights(length int) []float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	for i := range length {
		weights[i] /= sum
	}
	c.cache[length] = weights
	return weights
}
