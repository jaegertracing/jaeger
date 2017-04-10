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

	agent := &AgentService{url: server.URL, logger: zap.NewNop()}
	rate, err := agent.GetSamplingRate("svc", "op")
	assert.NoError(t, err)
	assert.EqualValues(t, 1, rate)

	rate, err = agent.GetSamplingRate("bad_svc", "op")
	assert.Error(t, err)
}
