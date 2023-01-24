// Copyright (c) 2023 The Jaeger Authors.
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

package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	thriftconv "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	api_v1 "github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

func TestSamplingStrategyResponseToJSON_Error(t *testing.T) {
	_, err := SamplingStrategyResponseToJSON(nil)
	assert.Error(t, err)
}

// TestSamplingStrategyResponseToJSON verifies that the function outputs
// the same string as Thrift-based JSON marshaler.
func TestSamplingStrategyResponseToJSON(t *testing.T) {
	t.Run("probabilistic", func(t *testing.T) {
		s := &api_v1.ProbabilisticSamplingStrategy{SamplingRate: 0.42}
		compareProtoAndThriftJSON(t, &api_v1.SamplingStrategyResponse{
			StrategyType:          api_v1.SamplingStrategyType_PROBABILISTIC,
			ProbabilisticSampling: s,
		})
	})
	t.Run("rateLimiting", func(t *testing.T) {
		s := &api_v1.RateLimitingSamplingStrategy{MaxTracesPerSecond: 42}
		compareProtoAndThriftJSON(t, &api_v1.SamplingStrategyResponse{
			StrategyType:         api_v1.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: s,
		})
	})
	t.Run("operationSampling", func(t *testing.T) {
		a := 11.2 // we need a pointer to value
		s := &api_v1.PerOperationSamplingStrategies{
			DefaultSamplingProbability:       0.42,
			DefaultUpperBoundTracesPerSecond: &a,
			DefaultLowerBoundTracesPerSecond: 2,
			PerOperationStrategies: []*api_v1.OperationSamplingStrategy{
				{Operation: "fao"},
			},
		}

		compareProtoAndThriftJSON(t, &api_v1.SamplingStrategyResponse{
			OperationSampling: s,
		})
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
