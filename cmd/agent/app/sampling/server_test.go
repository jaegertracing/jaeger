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

package sampling

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"
	"github.com/uber/jaeger/thrift-gen/sampling"

	tSampling092 "github.com/uber/jaeger/cmd/agent/app/sampling/thrift-0.9.2"
)

type testServer struct {
	metricsFactory *metrics.LocalFactory
	mgr            *mockManager
	server         *httptest.Server
}

func withServer(
	mockResponse *sampling.SamplingStrategyResponse,
	runTest func(server *testServer),
) {
	metricsFactory := metrics.NewLocalFactory(0)
	mgr := &mockManager{response: mockResponse}
	realServer := NewSamplingServer(":0", mgr, metricsFactory)
	server := httptest.NewServer(realServer.Handler)
	defer server.Close()
	runTest(&testServer{
		metricsFactory: metricsFactory,
		mgr:            mgr,
		server:         server,
	})
}

func TestSamplingHandler(t *testing.T) {
	withServer(probabilistic(0.001), func(ts *testServer) {
		for _, endpoint := range []string{"/", "/sampling"} {
			t.Run("request against endpoint "+endpoint, func(t *testing.T) {
				resp, err := http.Get(ts.server.URL + endpoint + "?service=Y")
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				body, err := ioutil.ReadAll(resp.Body)
				assert.NoError(t, err)
				if endpoint == "/" {
					objResp := &tSampling092.SamplingStrategyResponse{}
					if assert.NoError(t, json.Unmarshal(body, objResp)) {
						assert.EqualValues(t,
							ts.mgr.response.GetStrategyType(),
							objResp.GetStrategyType())
						assert.Equal(t,
							ts.mgr.response.GetProbabilisticSampling().GetSamplingRate(),
							objResp.GetProbabilisticSampling().GetSamplingRate(),
						)
					}
				} else {
					objResp := &sampling.SamplingStrategyResponse{}
					if assert.NoError(t, json.Unmarshal(body, objResp)) {
						assert.EqualValues(t, ts.mgr.response, objResp)
					}
				}
			})
		}

		// handler must emit metrics
		testutils.AssertCounterMetrics(t, ts.metricsFactory, []testutils.ExpectedMetric{
			{Name: "sampling-server.requests", Value: 1},
			{Name: "sampling-server.requests-thrift-092", Value: 1},
		}...)
	})
}

func TestSamplingHandlerErrors(t *testing.T) {
	testCases := []struct {
		description  string
		mockResponse *sampling.SamplingStrategyResponse
		url          string
		statusCode   int
		body         string
		metrics      []mTestutils.ExpectedMetric
	}{
		{
			description: "no service name",
			url:         "",
			statusCode:  http.StatusBadRequest,
			body:        "'service' parameter is empty\n",
			metrics: []mTestutils.ExpectedMetric{
				{Name: "sampling-server.bad-requests", Value: 1},
			},
		},
		{
			description: "too many service names",
			url:         "?service=Y&service=Y",
			statusCode:  http.StatusBadRequest,
			body:        "'service' parameter must occur only once\n",
			metrics: []mTestutils.ExpectedMetric{
				{Name: "sampling-server.bad-requests", Value: 1},
			},
		},
		{
			description: "tcollector error",
			url:         "?service=Y",
			statusCode:  http.StatusInternalServerError,
			body:        "tcollector error: no mock response provided\n",
			metrics: []mTestutils.ExpectedMetric{
				{Name: "sampling-server.bad-server-responses", Value: 1},
			},
		},
		{
			description:  "marshalling error",
			mockResponse: probabilistic(math.NaN()),
			url:          "?service=Y",
			statusCode:   http.StatusInternalServerError,
			body:         "Cannot marshall Thrift to JSON\n",
			metrics: []mTestutils.ExpectedMetric{
				{Name: "sampling-server.bad-thrift", Value: 1},
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.description, func(t *testing.T) {
			withServer(testCase.mockResponse, func(ts *testServer) {
				resp, err := http.Get(ts.server.URL + testCase.url)
				assert.NoError(t, err)
				assert.Equal(t, testCase.statusCode, resp.StatusCode)
				if testCase.body != "" {
					body, err := ioutil.ReadAll(resp.Body)
					assert.NoError(t, err)
					assert.Equal(t, testCase.body, string(body))
				}

				if len(testCase.metrics) > 0 {
					mTestutils.AssertCounterMetrics(t, ts.metricsFactory, testCase.metrics...)
				}
			})
		})
	}

	t.Run("failure to write a response", func(t *testing.T) {
		withServer(probabilistic(0.001), func(ts *testServer) {
			handler := newSamplingHandler(ts.mgr, ts.metricsFactory)

			req := httptest.NewRequest("GET", "http://localhost:80/?service=X", nil)
			w := &mockWriter{header: make(http.Header)}
			handler.serveHTTP(w, req, false)

			mTestutils.AssertCounterMetrics(t, ts.metricsFactory,
				mTestutils.ExpectedMetric{Name: "sampling-server.write-errors", Value: 1})
		})
	})
}

func probabilistic(probability float64) *sampling.SamplingStrategyResponse {
	return &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: probability,
		},
	}
}

type mockWriter struct {
	header http.Header
}

func (w *mockWriter) Header() http.Header {
	return w.header
}

func (w *mockWriter) Write([]byte) (int, error) {
	return 0, errors.New("write error")
}

func (w *mockWriter) WriteHeader(int) {}

type mockManager struct {
	response *sampling.SamplingStrategyResponse
}

func (m *mockManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	if m.response == nil {
		return nil, errors.New("no mock response provided")
	}
	return m.response, nil
}
