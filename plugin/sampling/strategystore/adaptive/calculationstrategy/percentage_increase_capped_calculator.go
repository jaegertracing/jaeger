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

const (
	defaultPercentageIncreaseCap = 0.5
)

// PercentageIncreaseCappedCalculator is a probability calculator that caps the probability
// increase to a certain percentage of the previous probability.
//
// Given prevProb = 0.1, newProb = 0.5, and cap = 0.5:
// (0.5 - 0.1)/0.1 = 400% increase. Given that our cap is 50%, the probability can only
// increase to 0.15.
//
// Given prevProb = 0.4, newProb = 0.5, and cap = 0.5:
// (0.5 - 0.4)/0.4 = 25% increase. Given that this is below our cap of 50%, the probability
// can increase to 0.5.
type PercentageIncreaseCappedCalculator struct {
	percentageIncreaseCap float64
}

// NewPercentageIncreaseCappedCalculator returns a new percentage increase capped calculator.
func NewPercentageIncreaseCappedCalculator(percentageIncreaseCap float64) PercentageIncreaseCappedCalculator {
	if percentageIncreaseCap == 0 {
		percentageIncreaseCap = defaultPercentageIncreaseCap
	}
	return PercentageIncreaseCappedCalculator{
		percentageIncreaseCap: percentageIncreaseCap,
	}
}

// Calculate calculates the new probability.
func (c PercentageIncreaseCappedCalculator) Calculate(targetQPS, curQPS, prevProbability float64) float64 {
	factor := targetQPS / curQPS
	newProbability := prevProbability * factor
	// If curQPS is lower than the targetQPS, we need to increase the probability slowly to
	// defend against oversampling.
	// Else if curQPS is higher than the targetQPS, jump directly to the newProbability to
	// defend against oversampling.
	if factor > 1.0 {
		percentIncrease := (newProbability - prevProbability) / prevProbability
		if percentIncrease > c.percentageIncreaseCap {
			newProbability = prevProbability + (prevProbability * c.percentageIncreaseCap)
		}
	}
	return newProbability
}
