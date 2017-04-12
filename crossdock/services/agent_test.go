// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/uber/jaeger/thrift-gen/sampling"
)

var (
	testResponse = &sampling.SamplingStrategyResponse{
		OperationSampling: &sampling.PerOperationSamplingStrategies{
			PerOperationStrategies: []*sampling.OperationSamplingStrategy{
				{
					Operation: "op",
					ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
						SamplingRate: 0.01,
					},
				},
			},
		},
	}
)

func TestGetSamplingRateInternal(t *testing.T) {
	tests := []struct {
		operation string
		response  *sampling.SamplingStrategyResponse
		shouldErr bool
		rate      float64
	}{
		{"op", &sampling.SamplingStrategyResponse{}, true, 0},
		{"op", &sampling.SamplingStrategyResponse{OperationSampling: &sampling.PerOperationSamplingStrategies{}}, true, 0},
		{"op", testResponse, false, 0.01},
		{"nop", testResponse, true, 0},
	}

	for _, test := range tests {
		rate, err := getSamplingRate(test.operation, test.response)
		if test.shouldErr {
			assert.EqualError(t, err, errSamplingRateMissing.Error())
		}
		assert.Equal(t, test.rate, rate)
	}
}

type testAgentHandler struct{}

func (h *testAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	svc := r.FormValue("service")
	body := []byte("bad json")
	if svc == "crossdock-svc" {
		response := sampling.SamplingStrategyResponse{
			OperationSampling: &sampling.PerOperationSamplingStrategies{
				PerOperationStrategies: []*sampling.OperationSamplingStrategy{
					{
						Operation: "op",
						ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
							SamplingRate: 1,
						},
					},
				},
			},
		}
		body, _ = json.Marshal(response)
	}
	w.Write(body)
}

func TestGetSamplingRate(t *testing.T) {
	handler := &testAgentHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	agent := NewAgentService("", zap.NewNop())
	rate, err := agent.GetSamplingRate("svc", "op")
	assert.Error(t, err)

	agent = NewAgentService(server.URL, zap.NewNop())
	rate, err = agent.GetSamplingRate("svc", "op")
	assert.NoError(t, err)
	assert.EqualValues(t, 1, rate)

	rate, err = agent.GetSamplingRate("bad_svc", "op")
	assert.Error(t, err)
}
