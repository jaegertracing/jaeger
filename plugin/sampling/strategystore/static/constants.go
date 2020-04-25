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

package static

import (
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
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
func defaultStrategyResponse() *sampling.SamplingStrategyResponse {
	return &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: defaultSamplingProbability,
		},
	}
}

func defaultStrategies() *storedStrategies {
	s := &storedStrategies{
		serviceStrategies: make(map[string]*sampling.SamplingStrategyResponse),
	}
	s.defaultStrategy = defaultStrategyResponse()
	return s
}
