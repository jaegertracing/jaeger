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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

func TestStrategyStore(t *testing.T) {
	_, err := NewStrategyStore(Options{StrategiesFile: "fileNotFound.json"}, zap.NewNop())
	assert.EqualError(t, err, "Failed to open strategies file: open fileNotFound.json: no such file or directory")

	_, err = NewStrategyStore(Options{StrategiesFile: "fixtures/bad_strategies.json"}, zap.NewNop())
	assert.EqualError(t, err,
		"Failed to unmarshal strategies: json: cannot unmarshal string into Go value of type static.strategies")

	// Test default strategy
	logger, buf := testutils.NewLogger()
	store, err := NewStrategyStore(Options{}, logger)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No sampling strategies provided, using defaults")
	s, err := store.GetSamplingStrategy("foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.001), *s)

	// Test reading strategies from a file
	store, err = NewStrategyStore(Options{StrategiesFile: "fixtures/strategies.json"}, logger)
	require.NoError(t, err)
	s, err = store.GetSamplingStrategy("foo")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8), *s)

	s, err = store.GetSamplingStrategy("bar")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 5), *s)

	s, err = store.GetSamplingStrategy("default")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.5), *s)
}

func TestPerOperationSamplingStrategies(t *testing.T) {
	logger, buf := testutils.NewLogger()
	store, err := NewStrategyStore(Options{StrategiesFile: "fixtures/operation_strategies.json"}, logger)
	assert.Contains(t, buf.String(), "Operation strategies only supports probabilistic sampling at the moment,"+
		"'op2' defaulting to probabilistic sampling with probability 0.8")
	assert.Contains(t, buf.String(), "Operation strategies only supports probabilistic sampling at the moment,"+
		"'op4' defaulting to probabilistic sampling with probability 0.001")
	require.NoError(t, err)

	expected := makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.8)

	s, err := store.GetSamplingStrategy("foo")
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, s.StrategyType)
	assert.Equal(t, *expected.ProbabilisticSampling, *s.ProbabilisticSampling)

	require.NotNil(t, s.OperationSampling)
	os := s.OperationSampling
	assert.EqualValues(t, os.DefaultSamplingProbability, 0.8)
	require.Len(t, os.PerOperationStrategies, 1)
	assert.Equal(t, "op1", os.PerOperationStrategies[0].Operation)
	assert.EqualValues(t, 0.2, os.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate)

	expected = makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 5)

	s, err = store.GetSamplingStrategy("bar")
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_RATE_LIMITING, s.StrategyType)
	assert.Equal(t, *expected.RateLimitingSampling, *s.RateLimitingSampling)

	require.NotNil(t, s.OperationSampling)
	os = s.OperationSampling
	assert.EqualValues(t, os.DefaultSamplingProbability, 0.001)
	require.Len(t, os.PerOperationStrategies, 1)
	assert.Equal(t, "op3", os.PerOperationStrategies[0].Operation)
	assert.EqualValues(t, 0.3, os.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate)

	s, err = store.GetSamplingStrategy("default")
	require.NoError(t, err)
	assert.EqualValues(t, makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.5), *s)
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		strategy serviceStrategy
		expected sampling.SamplingStrategyResponse
	}{
		{
			strategy: serviceStrategy{
				Service:  "svc",
				strategy: strategy{Type: "probabilistic", Param: 0.2},
			},
			expected: makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.2),
		},
		{
			strategy: serviceStrategy{
				Service:  "svc",
				strategy: strategy{Type: "ratelimiting", Param: 3.5},
			},
			expected: makeResponse(sampling.SamplingStrategyType_RATE_LIMITING, 3),
		},
	}
	logger, buf := testutils.NewLogger()
	store := &strategyStore{logger: logger}
	for _, test := range tests {
		tt := test
		t.Run("", func(t *testing.T) {
			assert.EqualValues(t, tt.expected, *store.parseStrategy(&tt.strategy.strategy))
		})
	}
	assert.Empty(t, buf.String())

	// Test nonexistent strategy type
	actual := *store.parseStrategy(&strategy{Type: "blah", Param: 3.5})
	expected := makeResponse(sampling.SamplingStrategyType_PROBABILISTIC, defaultSamplingProbability)
	assert.EqualValues(t, expected, actual)
	assert.Contains(t, buf.String(), "Failed to parse sampling strategy")
}

func makeResponse(samplerType sampling.SamplingStrategyType, param float64) (resp sampling.SamplingStrategyResponse) {
	resp.StrategyType = samplerType
	if samplerType == sampling.SamplingStrategyType_PROBABILISTIC {
		resp.ProbabilisticSampling = &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: param,
		}
	} else if samplerType == sampling.SamplingStrategyType_RATE_LIMITING {
		resp.RateLimitingSampling = &sampling.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: int16(param),
		}
	}
	return resp
}
