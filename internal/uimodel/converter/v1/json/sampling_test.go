// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	apiv1 "github.com/jaegertracing/jaeger-idl/thrift-gen/sampling"
	thriftconv "github.com/jaegertracing/jaeger/internal/converter/thrift/jaeger"
)

func TestSamplingStrategyResponseToJSON_Error(t *testing.T) {
	_, err := SamplingStrategyResponseToJSON(nil)
	require.Error(t, err)
}

// TestSamplingStrategyResponseToJSON verifies that the function outputs
// the same string as Thrift-based JSON marshaler.
func TestSamplingStrategyResponseToJSON(t *testing.T) {
	t.Run("probabilistic", func(t *testing.T) {
		s := &apiv1.SamplingStrategyResponse{
			StrategyType: apiv1.SamplingStrategyType_PROBABILISTIC,
			ProbabilisticSampling: &apiv1.ProbabilisticSamplingStrategy{
				SamplingRate: 0.42,
			},
		}
		compareProtoAndThriftJSON(t, s)
	})
	t.Run("rateLimiting", func(t *testing.T) {
		s := &apiv1.SamplingStrategyResponse{
			StrategyType: apiv1.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &apiv1.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: 42,
			},
		}
		compareProtoAndThriftJSON(t, s)
	})
	t.Run("operationSampling", func(t *testing.T) {
		a := 11.2 // we need a pointer to value
		s := &apiv1.SamplingStrategyResponse{
			OperationSampling: &apiv1.PerOperationSamplingStrategies{
				DefaultSamplingProbability:       0.42,
				DefaultUpperBoundTracesPerSecond: &a,
				DefaultLowerBoundTracesPerSecond: 2,
				PerOperationStrategies: []*apiv1.OperationSamplingStrategy{
					{
						Operation: "foo",
						ProbabilisticSampling: &apiv1.ProbabilisticSamplingStrategy{
							SamplingRate: 0.42,
						},
					},
					{
						Operation: "bar",
						ProbabilisticSampling: &apiv1.ProbabilisticSamplingStrategy{
							SamplingRate: 0.42,
						},
					},
				},
			},
		}
		compareProtoAndThriftJSON(t, s)
	})
}

func compareProtoAndThriftJSON(t *testing.T, thriftObj *apiv1.SamplingStrategyResponse) {
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
	assert.Equal(t, s1.GetProbabilisticSampling(), s2.GetProbabilisticSampling())
}
