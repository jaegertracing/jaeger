// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaeger

import (
	"errors"
	"math"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/sampling"
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
	thriftResp := &sampling.SamplingStrategyResponse{
		StrategyType:          typ,
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
	return &sampling.RateLimitingSamplingStrategy{
		//nolint: gosec // G115
		MaxTracesPerSecond: int16(s.GetMaxTracesPerSecond()),
	}, nil
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

	perOp := s.GetPerOperationStrategies()
	// Default to empty array so that json.Marshal returns [] instead of null (Issue #3891).
	r.PerOperationStrategies = make([]*sampling.OperationSamplingStrategy, len(perOp))
	for i, k := range perOp {
		r.PerOperationStrategies[i] = convertOperationFromDomain(k)
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
