// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

// parseSamplingResponse parses a JSON sampling strategy response
// using the same jsonpb.Unmarshal logic as the OTel Jaeger Remote Sampler SDK.
// See: https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/samplers/jaegerremote/sampler_remote.go
func parseSamplingResponse(t *testing.T, body []byte) *api_v2.SamplingStrategyResponse {
	t.Helper()
	strategy := new(api_v2.SamplingStrategyResponse)
	require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(body), strategy))
	return strategy
}

type testServer struct {
	metricsFactory   *metricstest.Factory
	samplingProvider *mockSamplingProvider
	server           *httptest.Server
	handler          *Handler
}

func withServer(
	mockSamplingResponse *api_v2.SamplingStrategyResponse,
	testFn func(server *testServer),
) {
	metricsFactory := metricstest.NewFactory(0)
	samplingProvider := &mockSamplingProvider{samplingResponse: mockSamplingResponse}
	cfgMgr := &ConfigManager{
		SamplingProvider: samplingProvider,
	}
	handler := NewHandler(HandlerParams{
		ConfigManager:  cfgMgr,
		MetricsFactory: metricsFactory,
	})

	httpMux := http.NewServeMux()
	handler.RegisterRoutes(httpMux)
	server := httptest.NewServer(httpMux)

	defer server.Close()
	testFn(&testServer{
		metricsFactory:   metricsFactory,
		samplingProvider: samplingProvider,
		server:           server,
		handler:          handler,
	})
}

func TestHTTPHandler(t *testing.T) {
	withServer(rateLimiting(42), func(ts *testServer) {
		resp, err := http.Get(ts.server.URL + "/?service=Y")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		assert.JSONEq(t,
			`{"strategyType":"RATE_LIMITING","rateLimitingSampling":{"maxTracesPerSecond":42}}`,
			string(body))
		objResp := parseSamplingResponse(t, body)
		assert.Equal(t, ts.samplingProvider.samplingResponse, objResp)

		// handler must emit metrics
		ts.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{Name: "http-server.requests", Tags: map[string]string{"type": "sampling"}, Value: 1})
	})
}

// TestOTelSDKCompatibility verifies that the response from
// the RegisterRoutes endpoint can be parsed by the same jsonpb.Unmarshal
// logic used in the OpenTelemetry Jaeger Remote Sampler SDK:
// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/samplers/jaegerremote/sampler_remote.go
//
// The SDK's Parse function does:
//
//	strategy := new(jaeger_api_v2.SamplingStrategyResponse)
//	if err := jsonpb.Unmarshal(bytes.NewReader(response), strategy); err != nil { ... }
//
// Gogo's jsonpb module can parse both string-based and numeric enum formats.
// Cf. https://github.com/open-telemetry/opentelemetry-go-contrib/issues/3184
func TestOTelSDKCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		response *api_v2.SamplingStrategyResponse
	}{
		{
			name:     "rate limiting strategy",
			response: rateLimiting(42),
		},
		{
			name:     "probabilistic strategy",
			response: probabilistic(0.5),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withServer(test.response, func(ts *testServer) {
				resp, err := http.Get(ts.server.URL + "/?service=Y")
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())

				// Parse the response the same way as the OTel Jaeger Remote Sampler SDK.
				// See: https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/samplers/jaegerremote/sampler_remote.go
				objResp := parseSamplingResponse(t, body)
				assert.Equal(t,
					test.response.GetStrategyType(),
					objResp.GetStrategyType())
				// even though one of these strategies is nil, the generated code
				// still allows to call next method on it and return default value.
				assert.InDelta(t,
					test.response.GetProbabilisticSampling().GetSamplingRate(),
					objResp.GetProbabilisticSampling().GetSamplingRate(),
					0)
				assert.Equal(t,
					test.response.GetRateLimitingSampling().GetMaxTracesPerSecond(),
					objResp.GetRateLimitingSampling().GetMaxTracesPerSecond())
			})
		})
	}
}

func TestHTTPHandlerErrors(t *testing.T) {
	testCases := []struct {
		description          string
		mockSamplingResponse *api_v2.SamplingStrategyResponse
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
			description: "sampler collector error",
			url:         "?service=Y",
			statusCode:  http.StatusInternalServerError,
			body:        "collector error: no mock response provided\n",
			metrics: []metricstest.ExpectedMetric{
				{Name: "http-server.errors", Tags: map[string]string{"source": "collector-proxy", "status": "5xx"}, Value: 1},
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.description, func(t *testing.T) {
			withServer(testCase.mockSamplingResponse, func(ts *testServer) {
				resp, err := http.Get(ts.server.URL + testCase.url)
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, testCase.statusCode, resp.StatusCode)
				if testCase.body != "" {
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					assert.Equal(t, testCase.body, string(body))
				}

				if len(testCase.metrics) > 0 {
					ts.metricsFactory.AssertCounterMetrics(t, testCase.metrics...)
				}
			})
		})
	}

	t.Run("failure to write a response", func(t *testing.T) {
		withServer(probabilistic(0.001), func(ts *testServer) {
			handler := ts.handler

			req := httptest.NewRequest(http.MethodGet, "http://localhost:80/?service=X", http.NoBody)
			w := &mockWriter{header: make(http.Header)}
			handler.serveSamplingHTTP(w, req, handler.encodeProto)

			ts.metricsFactory.AssertCounterMetrics(t,
				metricstest.ExpectedMetric{Name: "http-server.errors", Tags: map[string]string{"source": "write", "status": "5xx"}, Value: 1})
		})
	})
}

func TestEncodeErrors(t *testing.T) {
	withServer(nil, func(server *testServer) {
		_, err := server.handler.encodeProto(nil)
		require.ErrorContains(t, err, "SamplingStrategyResponseToJSON failed")
		server.metricsFactory.AssertCounterMetrics(t, []metricstest.ExpectedMetric{
			{Name: "http-server.errors", Tags: map[string]string{"source": "proto", "status": "5xx"}, Value: 1},
		}...)
	})
}

func rateLimiting(rate int32) *api_v2.SamplingStrategyResponse {
	return &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_RATE_LIMITING,
		RateLimitingSampling: &api_v2.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: rate,
		},
	}
}

func probabilistic(probability float64) *api_v2.SamplingStrategyResponse {
	return &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
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

func (*mockWriter) Write([]byte) (int, error) {
	return 0, errors.New("write error")
}

func (*mockWriter) WriteHeader(int) {}
