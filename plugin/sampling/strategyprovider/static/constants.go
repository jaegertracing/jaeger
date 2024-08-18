// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package static

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

	// defaultSamplingProbability is the default sampling probability the
	// Strategy Store will use if none is provided.
	defaultSamplingProbability = 0.001
)

// defaultStrategy is the default sampling strategy the Strategy Store will return
// if none is provided.
func defaultStrategyResponse() *api_v2.SamplingStrategyResponse {
	return &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
			SamplingRate: defaultSamplingProbability,
		},
	}
}

func defaultStrategies() *storedStrategies {
	s := &storedStrategies{
		serviceStrategies: make(map[string]*api_v2.SamplingStrategyResponse),
	}
	s.defaultStrategy = defaultStrategyResponse()
	return s
}
