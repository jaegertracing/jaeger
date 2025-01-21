// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

const (
	// samplerTypeProbabilistic is the type of sampler that samples traces
	// with a certain fixed probability.
	samplerTypeProbabilistic = "probabilistic"

	// samplerTypeRateLimiting is the type of sampler that samples
	// only up to a fixed number of traces per second.
	samplerTypeRateLimiting = "ratelimiting"

	// DefaultSamplingProbability is the default value for  "DefaultSamplingProbability"
	// used by the Strategy Store in case no DefaultSamplingProbability is defined
	DefaultSamplingProbability = 0.001
)

// defaultStrategy is the default sampling strategy the Strategy Store will return
// if none is provided.
func defaultStrategyResponse(defaultSamplingProbability float64) *api_v2.SamplingStrategyResponse {
	return &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
			SamplingRate: defaultSamplingProbability,
		},
	}
}

func defaultStrategies(defaultSamplingProbability float64) *storedStrategies {
	s := &storedStrategies{
		serviceStrategies: make(map[string]*api_v2.SamplingStrategyResponse),
	}
	s.defaultStrategy = defaultStrategyResponse(defaultSamplingProbability)
	return s
}
