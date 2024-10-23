// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
		assert.InDelta(t, test.rate, rate, 0.01)
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
	agent := NewCollectorService("", zap.NewNop())
	_, err := agent.GetSamplingRate("svc", "op")
	require.Error(t, err)

	agent = NewCollectorService(server.URL, zap.NewNop())
	rate, err := agent.GetSamplingRate("svc", "op")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, rate, 0.01)

	_, err = agent.GetSamplingRate("bad_svc", "op")
	require.Error(t, err)
}

func TestGetSamplingRateReadAllErr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1")
	}))
	defer server.Close()
	agent := NewCollectorService(server.URL, zap.NewNop())
	_, err := agent.GetSamplingRate("svc", "op")
	require.EqualError(t, err, "unexpected EOF")
}
