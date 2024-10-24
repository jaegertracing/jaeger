// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	thriftconv "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	api_v1 "github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

func TestSamplingStrategyResponseToJSON_Error(t *testing.T) {
	_, err := SamplingStrategyResponseToJSON(nil)
	require.Error(t, err)
}

// TestSamplingStrategyResponseToJSON verifies that the function outputs
// the same string as Thrift-based JSON marshaler.
func TestSamplingStrategyResponseToJSON(t *testing.T) {
	t.Run("probabilistic", func(t *testing.T) {
		s := &api_v1.SamplingStrategyResponse{
			StrategyType: api_v1.SamplingStrategyType_PROBABILISTIC,
			ProbabilisticSampling: &api_v1.ProbabilisticSamplingStrategy{
				SamplingRate: 0.42,
			},
		}
		compareProtoAndThriftJSON(t, s)
	})
	t.Run("rateLimiting", func(t *testing.T) {
		s := &api_v1.SamplingStrategyResponse{
			StrategyType: api_v1.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &api_v1.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: 42,
			},
		}
		compareProtoAndThriftJSON(t, s)
	})
	t.Run("operationSampling", func(t *testing.T) {
		a := 11.2 // we need a pointer to value
		s := &api_v1.SamplingStrategyResponse{
			OperationSampling: &api_v1.PerOperationSamplingStrategies{
				DefaultSamplingProbability:       0.42,
				DefaultUpperBoundTracesPerSecond: &a,
				DefaultLowerBoundTracesPerSecond: 2,
				PerOperationStrategies: []*api_v1.OperationSamplingStrategy{
					{
						Operation: "foo",
						ProbabilisticSampling: &api_v1.ProbabilisticSamplingStrategy{
							SamplingRate: 0.42,
						},
					},
					{
						Operation: "bar",
						ProbabilisticSampling: &api_v1.ProbabilisticSamplingStrategy{
							SamplingRate: 0.42,
						},
					},
				},
			},
		}
		compareProtoAndThriftJSON(t, s)
	})
}

func compareProtoAndThriftJSON(t *testing.T, thriftObj *api_v1.SamplingStrategyResponse) {
	protoObj, err := thriftconv.ConvertSamplingResponseToDomain(thriftObj)
	require.NoError(t, err)

	s1, err := json.Marshal(thriftObj)
	require.NoError(t, err)

	s2, err := SamplingStrategyResponseToJSON(protoObj)
	require.NoError(t, err)

	assert.Equal(t, string(s1), s2)
}

func TestSamplingStrategyResponseFromJSON(t *testing.T) {
	_, err := SamplingStrategyResponseFromJSON([]byte("broken"))
	require.Error(t, err)

	s1 := &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
			SamplingRate: 0.42,
		},
	}
	jsonData, err := SamplingStrategyResponseToJSON(s1)
	require.NoError(t, err)

	s2, err := SamplingStrategyResponseFromJSON([]byte(jsonData))
	require.NoError(t, err)
	assert.Equal(t, s1.GetStrategyType(), s2.GetStrategyType())
	assert.EqualValues(t, s1.GetProbabilisticSampling(), s2.GetProbabilisticSampling())
}
