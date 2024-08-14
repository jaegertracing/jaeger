// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package calculationstrategy

// ProbabilityCalculator calculates the new probability given the current and target QPS
type ProbabilityCalculator interface {
	Calculate(targetQPS, curQPS, prevProbability float64) (newProbability float64)
}

// CalculateFunc wraps a function of appropriate signature and makes a ProbabilityCalculator from it.
type CalculateFunc func(targetQPS, curQPS, prevProbability float64) (newProbability float64)

// Calculate implements Calculator interface.
func (c CalculateFunc) Calculate(targetQPS, curQPS, prevProbability float64) float64 {
	return c(targetQPS, curQPS, prevProbability)
}
