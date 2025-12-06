// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package calculationstrategy

// ProbabilityCalculator calculates the new probability given the current and target QPS
type ProbabilityCalculator interface {
	Calculate(targetQPS, curQPS, prevProbability float64) (newProbability float64)
}
