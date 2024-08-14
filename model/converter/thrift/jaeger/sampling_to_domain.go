// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaeger

import (
	"errors"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// ConvertSamplingResponseToDomain converts thrift sampling response to its proto representation.
func ConvertSamplingResponseToDomain(r *sampling.SamplingStrategyResponse) (*api_v2.SamplingStrategyResponse, error) {
	if r == nil {
		return nil, nil
	}
	t, err := convertStrategyTypeToDomain(r.GetStrategyType())
	if err != nil {
		return nil, err
	}
	response := &api_v2.SamplingStrategyResponse{
		StrategyType:          t,
		ProbabilisticSampling: convertProbabilisticToDomain(r.GetProbabilisticSampling()),
		RateLimitingSampling:  convertRateLimitingToDomain(r.GetRateLimitingSampling()),
		OperationSampling:     convertPerOperationToDomain(r.GetOperationSampling()),
	}
	return response, nil
}

func convertRateLimitingToDomain(s *sampling.RateLimitingSamplingStrategy) *api_v2.RateLimitingSamplingStrategy {
	if s == nil {
		return nil
	}
	return &api_v2.RateLimitingSamplingStrategy{MaxTracesPerSecond: int32(s.GetMaxTracesPerSecond())}
}

func convertProbabilisticToDomain(s *sampling.ProbabilisticSamplingStrategy) *api_v2.ProbabilisticSamplingStrategy {
	if s == nil {
		return nil
	}
	return &api_v2.ProbabilisticSamplingStrategy{SamplingRate: s.GetSamplingRate()}
}

func convertPerOperationToDomain(s *sampling.PerOperationSamplingStrategies) *api_v2.PerOperationSamplingStrategies {
	if s == nil {
		return nil
	}
	poss := make([]*api_v2.OperationSamplingStrategy, len(s.PerOperationStrategies))
	for i, pos := range s.PerOperationStrategies {
		poss[i] = &api_v2.OperationSamplingStrategy{
			Operation:             pos.Operation,
			ProbabilisticSampling: convertProbabilisticToDomain(pos.GetProbabilisticSampling()),
		}
	}
	return &api_v2.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       s.GetDefaultSamplingProbability(),
		DefaultUpperBoundTracesPerSecond: s.GetDefaultUpperBoundTracesPerSecond(),
		DefaultLowerBoundTracesPerSecond: s.GetDefaultLowerBoundTracesPerSecond(),
		PerOperationStrategies:           poss,
	}
}

func convertStrategyTypeToDomain(t sampling.SamplingStrategyType) (api_v2.SamplingStrategyType, error) {
	switch t {
	case sampling.SamplingStrategyType_PROBABILISTIC:
		return api_v2.SamplingStrategyType_PROBABILISTIC, nil
	case sampling.SamplingStrategyType_RATE_LIMITING:
		return api_v2.SamplingStrategyType_RATE_LIMITING, nil
	default:
		return api_v2.SamplingStrategyType_PROBABILISTIC, errors.New("could not convert sampling strategy type")
	}
}
