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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

func TestConvertStrategyTypeToDomain(t *testing.T) {
	tests := []struct {
		in       sampling.SamplingStrategyType
		expected api_v2.SamplingStrategyType
		err      error
	}{
		{in: sampling.SamplingStrategyType_PROBABILISTIC, expected: api_v2.SamplingStrategyType_PROBABILISTIC},
		{in: sampling.SamplingStrategyType_RATE_LIMITING, expected: api_v2.SamplingStrategyType_RATE_LIMITING},
		{in: 44, err: errors.New("could not convert sampling strategy type")},
	}
	for _, test := range tests {
		st, err := convertStrategyTypeToDomain(test.in)
		if test.err != nil {
			assert.EqualError(t, test.err, err.Error())
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, st)
		}
	}
}

func TestConvertProbabilisticToDomain(t *testing.T) {
	tests := []struct {
		expected *api_v2.ProbabilisticSamplingStrategy
		in       *sampling.ProbabilisticSamplingStrategy
	}{
		{expected: &api_v2.ProbabilisticSamplingStrategy{SamplingRate: 21}, in: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 21}},
		{},
	}
	for _, test := range tests {
		st := convertProbabilisticToDomain(test.in)
		assert.Equal(t, test.expected, st)
	}
}

func TestConvertRateLimitingToDomain(t *testing.T) {
	tests := []struct {
		expected *api_v2.RateLimitingSamplingStrategy
		in       *sampling.RateLimitingSamplingStrategy
	}{
		{expected: &api_v2.RateLimitingSamplingStrategy{MaxTracesPerSecond: 21}, in: &sampling.RateLimitingSamplingStrategy{MaxTracesPerSecond: 21}},
		{},
	}
	for _, test := range tests {
		st := convertRateLimitingToDomain(test.in)
		assert.Equal(t, test.expected, st)
	}
}

func TestConvertPerOperationStrategyToDomain(t *testing.T) {
	var a = 11.2
	tests := []struct {
		expected *api_v2.PerOperationSamplingStrategies
		in       *sampling.PerOperationSamplingStrategies
	}{
		{expected: &api_v2.PerOperationSamplingStrategies{DefaultSamplingProbability: 15.2, DefaultUpperBoundTracesPerSecond: a, DefaultLowerBoundTracesPerSecond: 2,
			PerOperationStrategies: []*api_v2.OperationSamplingStrategy{{Operation: "fao"}}},
			in: &sampling.PerOperationSamplingStrategies{DefaultSamplingProbability: 15.2, DefaultUpperBoundTracesPerSecond: &a, DefaultLowerBoundTracesPerSecond: 2,
				PerOperationStrategies: []*sampling.OperationSamplingStrategy{{Operation: "fao"}}}},
		{},
	}
	for _, test := range tests {
		o := convertPerOperationToDomain(test.in)
		assert.Equal(t, test.expected, o)
	}
}

func TestConvertSamplingResponseToDomain(t *testing.T) {
	tests := []struct {
		expected *api_v2.SamplingStrategyResponse
		in       *sampling.SamplingStrategyResponse
		err      string
	}{
		{in: &sampling.SamplingStrategyResponse{StrategyType: 55}, err: "could not convert sampling strategy type"},
		{expected: &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC}, in: &sampling.SamplingStrategyResponse{StrategyType: sampling.SamplingStrategyType_PROBABILISTIC}},
		{},
	}
	for _, test := range tests {
		r, err := ConvertSamplingResponseToDomain(test.in)
		if test.err != "" {
			assert.EqualError(t, err, test.err)
			require.Nil(t, r)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, r)
		}
	}
}
