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

package jaeger

import (
	"errors"
	"math"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// ConvertSamplingResponseFromDomain converts proto sampling response to its thrift representation.
func ConvertSamplingResponseFromDomain(r *api_v2.SamplingStrategyResponse) (*sampling.SamplingStrategyResponse, error) {
	typ, err := convertStrategyTypeFromDomain(r.GetStrategyType())
	if err != nil {
		return nil, err
	}
	rl, err := convertRateLimitingFromDomain(r.GetRateLimitingSampling())
	if err != nil {
		return nil, err
	}
	thriftResp := &sampling.SamplingStrategyResponse{StrategyType: typ,
		ProbabilisticSampling: convertProbabilisticFromDomain(r.GetProbabilisticSampling()),
		RateLimitingSampling:  rl,
		OperationSampling:     convertPerOperationFromDomain(r.GetOperationSampling()),
	}
	return thriftResp, nil
}

func convertProbabilisticFromDomain(s *api_v2.ProbabilisticSamplingStrategy) *sampling.ProbabilisticSamplingStrategy {
	if s == nil {
		return nil
	}
	return &sampling.ProbabilisticSamplingStrategy{SamplingRate: s.GetSamplingRate()}
}

func convertRateLimitingFromDomain(s *api_v2.RateLimitingSamplingStrategy) (*sampling.RateLimitingSamplingStrategy, error) {
	if s == nil {
		return nil, nil
	}
	if s.MaxTracesPerSecond > math.MaxInt16 {
		return nil, errors.New("maxTracesPerSecond is higher than int16")
	}
	return &sampling.RateLimitingSamplingStrategy{MaxTracesPerSecond: int16(s.GetMaxTracesPerSecond())}, nil
}

func convertPerOperationFromDomain(s *api_v2.PerOperationSamplingStrategies) *sampling.PerOperationSamplingStrategies {
	if s == nil {
		return nil
	}
	r := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       s.GetDefaultSamplingProbability(),
		DefaultLowerBoundTracesPerSecond: s.GetDefaultLowerBoundTracesPerSecond(),
		DefaultUpperBoundTracesPerSecond: &s.DefaultUpperBoundTracesPerSecond,
	}
	if s.GetPerOperationStrategies() != nil {
		r.PerOperationStrategies = make([]*sampling.OperationSamplingStrategy, len(s.GetPerOperationStrategies()))
		for i, k := range s.PerOperationStrategies {
			r.PerOperationStrategies[i] = convertOperationFromDomain(k)
		}
	}
	return r
}

func convertOperationFromDomain(s *api_v2.OperationSamplingStrategy) *sampling.OperationSamplingStrategy {
	if s == nil {
		return nil
	}
	return &sampling.OperationSamplingStrategy{
		Operation:             s.GetOperation(),
		ProbabilisticSampling: convertProbabilisticFromDomain(s.GetProbabilisticSampling()),
	}
}

func convertStrategyTypeFromDomain(s api_v2.SamplingStrategyType) (sampling.SamplingStrategyType, error) {
	switch s {
	case api_v2.SamplingStrategyType_PROBABILISTIC:
		return sampling.SamplingStrategyType_PROBABILISTIC, nil
	case api_v2.SamplingStrategyType_RATE_LIMITING:
		return sampling.SamplingStrategyType_RATE_LIMITING, nil
	default:
		return sampling.SamplingStrategyType_PROBABILISTIC, errors.New("could not convert sampling strategy type")
	}
}
