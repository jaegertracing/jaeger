// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaeger

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/sampling"
)

func TestConvertStrategyTypeFromDomain(t *testing.T) {
	tests := []struct {
		expected sampling.SamplingStrategyType
		in       api_v2.SamplingStrategyType
		err      string
	}{
		{expected: sampling.SamplingStrategyType_PROBABILISTIC, in: api_v2.SamplingStrategyType_PROBABILISTIC},
		{expected: sampling.SamplingStrategyType_RATE_LIMITING, in: api_v2.SamplingStrategyType_RATE_LIMITING},
		{in: 44, err: "could not convert sampling strategy type"},
	}
	for _, test := range tests {
		st, err := convertStrategyTypeFromDomain(test.in)
		if test.err != "" {
			require.EqualError(t, err, test.err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, st)
		}
	}
}

func TestConvertProbabilisticFromDomain(t *testing.T) {
	tests := []struct {
		in       *api_v2.ProbabilisticSamplingStrategy
		expected *sampling.ProbabilisticSamplingStrategy
	}{
		{in: &api_v2.ProbabilisticSamplingStrategy{SamplingRate: 21}, expected: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 21}},
		{},
	}
	for _, test := range tests {
		st := convertProbabilisticFromDomain(test.in)
		assert.Equal(t, test.expected, st)
	}
}

func TestConvertRateLimitingFromDomain(t *testing.T) {
	tests := []struct {
		in       *api_v2.RateLimitingSamplingStrategy
		expected *sampling.RateLimitingSamplingStrategy
		err      string
	}{
		{in: &api_v2.RateLimitingSamplingStrategy{MaxTracesPerSecond: 21}, expected: &sampling.RateLimitingSamplingStrategy{MaxTracesPerSecond: 21}},
		{in: &api_v2.RateLimitingSamplingStrategy{MaxTracesPerSecond: math.MaxInt32}, err: "maxTracesPerSecond is higher than int16"},
		{},
	}
	for _, test := range tests {
		st, err := convertRateLimitingFromDomain(test.in)
		if test.err != "" {
			require.EqualError(t, err, test.err)
			require.Nil(t, st)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, st)
		}
	}
}

func TestConvertOperationStrategyFromDomain(t *testing.T) {
	tests := []struct {
		in       *api_v2.OperationSamplingStrategy
		expected *sampling.OperationSamplingStrategy
	}{
		{in: &api_v2.OperationSamplingStrategy{Operation: "foo"}, expected: &sampling.OperationSamplingStrategy{Operation: "foo"}},
		{
			in:       &api_v2.OperationSamplingStrategy{Operation: "foo", ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{SamplingRate: 2}},
			expected: &sampling.OperationSamplingStrategy{Operation: "foo", ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 2}},
		},
		{},
	}
	for _, test := range tests {
		o := convertOperationFromDomain(test.in)
		assert.Equal(t, test.expected, o)
	}
}

func TestConvertPerOperationStrategyFromDomain(t *testing.T) {
	a := 11.2
	tests := []struct {
		in       *api_v2.PerOperationSamplingStrategies
		expected *sampling.PerOperationSamplingStrategies
	}{
		{
			in: &api_v2.PerOperationSamplingStrategies{
				DefaultSamplingProbability: 15.2, DefaultUpperBoundTracesPerSecond: a, DefaultLowerBoundTracesPerSecond: 2,
				PerOperationStrategies: []*api_v2.OperationSamplingStrategy{{Operation: "fao"}},
			},
			expected: &sampling.PerOperationSamplingStrategies{
				DefaultSamplingProbability: 15.2, DefaultUpperBoundTracesPerSecond: &a, DefaultLowerBoundTracesPerSecond: 2,
				PerOperationStrategies: []*sampling.OperationSamplingStrategy{{Operation: "fao"}},
			},
		},
		{
			in: &api_v2.PerOperationSamplingStrategies{DefaultSamplingProbability: 15.2, DefaultUpperBoundTracesPerSecond: a, DefaultLowerBoundTracesPerSecond: 2},
			expected: &sampling.PerOperationSamplingStrategies{
				DefaultSamplingProbability: 15.2, DefaultUpperBoundTracesPerSecond: &a, DefaultLowerBoundTracesPerSecond: 2,
				PerOperationStrategies: []*sampling.OperationSamplingStrategy{},
			},
		},
	}
	for _, test := range tests {
		o := convertPerOperationFromDomain(test.in)
		assert.Equal(t, test.expected, o)
	}
}

func TestConvertSamplingResponseFromDomain(t *testing.T) {
	tests := []struct {
		in       *api_v2.SamplingStrategyResponse
		expected *sampling.SamplingStrategyResponse
		err      string
	}{
		{in: &api_v2.SamplingStrategyResponse{StrategyType: 55}, err: "could not convert sampling strategy type"},
		{
			in:  &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC, RateLimitingSampling: &api_v2.RateLimitingSamplingStrategy{MaxTracesPerSecond: math.MaxInt32}},
			err: "maxTracesPerSecond is higher than int16",
		},
		{in: &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC}, expected: &sampling.SamplingStrategyResponse{StrategyType: sampling.SamplingStrategyType_PROBABILISTIC}},
	}
	for _, test := range tests {
		r, err := ConvertSamplingResponseFromDomain(test.in)
		if test.err != "" {
			require.EqualError(t, err, test.err)
			require.Nil(t, r)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, r)
		}
	}
}
