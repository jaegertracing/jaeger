// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package services

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	p2json "github.com/jaegertracing/jaeger/model/converter/json"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

var testResponse = &api_v2.SamplingStrategyResponse{
	OperationSampling: &api_v2.PerOperationSamplingStrategies{
		PerOperationStrategies: []*api_v2.OperationSamplingStrategy{
			{
				Operation: "op",
				ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
					SamplingRate: 0.01,
				},
			},
		},
	},
}

func TestGetSamplingRateInternal(t *testing.T) {
	tests := []struct {
		operation string
		response  *api_v2.SamplingStrategyResponse
		shouldErr bool
		rate      float64
	}{
		{"op", &api_v2.SamplingStrategyResponse{}, true, 0},
		{"op", &api_v2.SamplingStrategyResponse{OperationSampling: &api_v2.PerOperationSamplingStrategies{}}, true, 0},
		{"op", testResponse, false, 0.01},
		{"nop", testResponse, true, 0},
	}

	for _, test := range tests {
		rate, err := getSamplingRate(test.operation, test.response)
		if test.shouldErr {
			require.EqualError(t, err, errSamplingRateMissing.Error())
		}
		assert.EqualValues(t, test.rate, rate)
	}
}

type testAgentHandler struct{}

func (*testAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	svc := r.FormValue("service")
	body := []byte("bad json")
	if svc == "crossdock-svc" {
		response := api_v2.SamplingStrategyResponse{
			OperationSampling: &api_v2.PerOperationSamplingStrategies{
				PerOperationStrategies: []*api_v2.OperationSamplingStrategy{
					{
						Operation: "op",
						ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
							SamplingRate: 1,
						},
					},
				},
			},
		}
		bodyStr, _ := p2json.SamplingStrategyResponseToJSON(&response)
		body = []byte(bodyStr)
	}
	w.Write(body)
}

func TestGetSamplingRate(t *testing.T) {
	handler := &testAgentHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test with no http server
	agent := NewAgentService("", zap.NewNop())
	_, err := agent.GetSamplingRate("svc", "op")
	require.Error(t, err)

	agent = NewAgentService(server.URL, zap.NewNop())
	rate, err := agent.GetSamplingRate("svc", "op")
	require.NoError(t, err)
	assert.EqualValues(t, 1, rate)

	_, err = agent.GetSamplingRate("bad_svc", "op")
	require.Error(t, err)
}

func TestGetSamplingRateReadAllErr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1")
	}))
	defer server.Close()
	agent := NewAgentService(server.URL, zap.NewNop())
	_, err := agent.GetSamplingRate("svc", "op")
	require.EqualError(t, err, "unexpected EOF")
}
