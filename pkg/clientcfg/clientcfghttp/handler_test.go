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

package clientcfghttp

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics/metricstest"

	tSampling092 "github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp/thrift-0.9.2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type testServer struct {
	metricsFactory *metricstest.Factory
	samplingStore  *mockSamplingStore
	bgMgr          *mockBaggageMgr
	server         *httptest.Server
	handler        *HTTPHandler
}

func withServer(
	basePath string,
	mockSamplingResponse *sampling.SamplingStrategyResponse,
	mockBaggageResponse []*baggage.BaggageRestriction,
	testFn func(server *testServer),
) {
	metricsFactory := metricstest.NewFactory(0)
	samplingStore := &mockSamplingStore{samplingResponse: mockSamplingResponse}
	bgMgr := &mockBaggageMgr{baggageResponse: mockBaggageResponse}
	cfgMgr := &ConfigManager{
		SamplingStrategyStore: samplingStore,
		BaggageManager:        bgMgr,
	}
	handler := NewHTTPHandler(HTTPHandlerParams{
		ConfigManager:          cfgMgr,
		MetricsFactory:         metricsFactory,
		BasePath:               basePath,
		LegacySamplingEndpoint: true,
	})
	r := mux.NewRouter()
	handler.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()
	testFn(&testServer{
		metricsFactory: metricsFactory,
		samplingStore:  samplingStore,
		bgMgr:          bgMgr,
		server:         server,
		handler:        handler,
	})
}

func TestHTTPHandler(t *testing.T) {
	testHTTPHandler(t, "")
	testHTTPHandler(t, "/foo")
}

func testHTTPHandler(t *testing.T, basePath string) {
	withServer(basePath, probabilistic(0.001), restrictions("luggage", 10), func(ts *testServer) {
		for _, endpoint := range []string{"/", "/sampling"} {
			t.Run("request against endpoint "+endpoint, func(t *testing.T) {
				resp, err := http.Get(ts.server.URL + basePath + endpoint + "?service=Y")
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				body, err := ioutil.ReadAll(resp.Body)
				require.NoError(t, err)
				err = resp.Body.Close()
				require.NoError(t, err)
				if endpoint == "/" {
					objResp := &tSampling092.SamplingStrategyResponse{}
					require.NoError(t, json.Unmarshal(body, objResp))
					assert.EqualValues(t,
						ts.samplingStore.samplingResponse.GetStrategyType(),
						objResp.GetStrategyType())
					assert.Equal(t,
						ts.samplingStore.samplingResponse.GetProbabilisticSampling().GetSamplingRate(),
						objResp.GetProbabilisticSampling().GetSamplingRate())
				} else {
					objResp := &sampling.SamplingStrategyResponse{}
					require.NoError(t, json.Unmarshal(body, objResp))
					assert.EqualValues(t, ts.samplingStore.samplingResponse, objResp)
				}
			})
		}

		t.Run("request against endpoint /baggageRestrictions", func(t *testing.T) {
			resp, err := http.Get(ts.server.URL + basePath + "/baggageRestrictions?service=Y")
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			var objResp []*baggage.BaggageRestriction
			require.NoError(t, json.Unmarshal(body, &objResp))
			assert.EqualValues(t, ts.bgMgr.baggageResponse, objResp)
		})

		// handler must emit metrics
		ts.metricsFactory.AssertCounterMetrics(t, []metricstest.ExpectedMetric{
			{Name: "http-server.requests", Tags: map[string]string{"type": "sampling"}, Value: 1},
			{Name: "http-server.requests", Tags: map[string]string{"type": "sampling-legacy"}, Value: 1},
			{Name: "http-server.requests", Tags: map[string]string{"type": "baggage"}, Value: 1},
		}...)
	})
}

func TestHTTPHandlerErrors(t *testing.T) {
	testCases := []struct {
		description          string
		mockSamplingResponse *sampling.SamplingStrategyResponse
		mockBaggageResponse  []*baggage.BaggageRestriction
		url                  string
		statusCode           int
		body                 string
		metrics              []metricstest.ExpectedMetric
	}{
		{
			description: "no service name",
			url:         "",
			statusCode:  http.StatusBadRequest,
			body:        "'service' parameter must be provided once\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "all", "status": "4xx"}, Value: 1},
			},
		},
		{
			description: "sampling endpoint too many service names",
			url:         "?service=Y&service=Y",
			statusCode:  http.StatusBadRequest,
			body:        "'service' parameter must be provided once\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "all", "status": "4xx"}, Value: 1},
			},
		},
		{
			description: "baggage endpoint too many service names",
			url:         "/baggageRestrictions?service=Y&service=Y",
			statusCode:  http.StatusBadRequest,
			body:        "'service' parameter must be provided once\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "all", "status": "4xx"}, Value: 1},
			},
		},
		{
			description: "sampler collector error",
			url:         "?service=Y",
			statusCode:  http.StatusInternalServerError,
			body:        "collector error: no mock response provided\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "collector-proxy", "status": "5xx"}, Value: 1},
			},
		},
		{
			description: "baggage collector error",
			url:         "/baggageRestrictions?service=Y",
			statusCode:  http.StatusInternalServerError,
			body:        "collector error: no mock response provided\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "collector-proxy", "status": "5xx"}, Value: 1},
			},
		},
		{
			description:          "sampler marshalling error",
			mockSamplingResponse: probabilistic(math.NaN()),
			url:                  "?service=Y",
			statusCode:           http.StatusInternalServerError,
			body:                 "cannot marshall Thrift to JSON\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "thrift", "status": "5xx"}, Value: 1},
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.description, func(t *testing.T) {
			withServer("", testCase.mockSamplingResponse, testCase.mockBaggageResponse, func(ts *testServer) {
				resp, err := http.Get(ts.server.URL + testCase.url)
				require.NoError(t, err)
				assert.Equal(t, testCase.statusCode, resp.StatusCode)
				if testCase.body != "" {
					body, err := ioutil.ReadAll(resp.Body)
					assert.NoError(t, err)
					assert.Equal(t, testCase.body, string(body))
				}

				if len(testCase.metrics) > 0 {
					ts.metricsFactory.AssertCounterMetrics(t, testCase.metrics...)
				}
			})
		})
	}

	t.Run("failure to write a response", func(t *testing.T) {
		withServer("", probabilistic(0.001), restrictions("luggage", 10), func(ts *testServer) {
			handler := ts.handler

			req := httptest.NewRequest("GET", "http://localhost:80/?service=X", nil)
			w := &mockWriter{header: make(http.Header)}
			handler.serveSamplingHTTP(w, req, false)

			ts.metricsFactory.AssertCounterMetrics(t,
				metricstest.ExpectedMetric{Name: "http-server.errors", Tags: map[string]string{"source": "write", "status": "5xx"}, Value: 1})

			req = httptest.NewRequest("GET", "http://localhost:80/baggageRestrictions?service=X", nil)
			handler.serveBaggageHTTP(w, req)

			ts.metricsFactory.AssertCounterMetrics(t,
				metricstest.ExpectedMetric{Name: "http-server.errors", Tags: map[string]string{"source": "write", "status": "5xx"}, Value: 2})
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

func restrictions(key string, size int32) []*baggage.BaggageRestriction {
	return []*baggage.BaggageRestriction{
		{BaggageKey: key, MaxValueLength: size},
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
