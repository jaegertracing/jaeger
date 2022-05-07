// Copyright (c) 2021 The Jaeger Authors.
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

package metricsstore

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

type (
	metricsTestCase struct {
		name             string
		serviceNames     []string
		spanKinds        []string
		groupByOperation bool
		wantName         string
		wantDescription  string
		wantLabels       map[string]string
		wantPromQlQuery  string
	}
)

const defaultTimeout = 30 * time.Second

func TestNewMetricsReaderValidAddress(t *testing.T) {
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, config.Configuration{
		ServerURL:      "http://localhost:1234",
		ConnectTimeout: defaultTimeout,
	})
	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestNewMetricsReaderInvalidAddress(t *testing.T) {
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, config.Configuration{
		ServerURL:      "\n",
		ConnectTimeout: defaultTimeout,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize prometheus client")
	assert.Nil(t, reader)
}

func TestGetMinStepDuration(t *testing.T) {
	params := metricsstore.MinStepDurationQueryParameters{}
	logger := zap.NewNop()
	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)

	reader, err := NewMetricsReader(logger, config.Configuration{
		ServerURL:      "http://" + listener.Addr().String(),
		ConnectTimeout: defaultTimeout,
	})
	require.NoError(t, err)

	minStep, err := reader.GetMinStepDuration(context.Background(), &params)
	require.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}

func TestMetricsServerError(t *testing.T) {
	endTime := time.Now()
	lookback := time.Minute
	step := time.Millisecond
	ratePer := 10 * time.Minute

	params := metricsstore.CallRateQueryParameters{
		BaseQueryParameters: metricsstore.BaseQueryParameters{
			EndTime:  &endTime,
			Lookback: &lookback,
			Step:     &step,
			RatePer:  &ratePer,
		},
	}

	mockPrometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer mockPrometheus.Close()

	logger := zap.NewNop()
	address := mockPrometheus.Listener.Addr().String()
	reader, err := NewMetricsReader(logger, config.Configuration{
		ServerURL:      "http://" + address,
		ConnectTimeout: defaultTimeout,
	})
	require.NoError(t, err)

	m, err := reader.GetCallRates(context.Background(), &params)
	assert.NotNil(t, m)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed executing metrics query")
}

func TestGetLatencies(t *testing.T) {
	for _, tc := range []metricsTestCase{
		{
			name:             "group by service should be reflected in name/description and query group-by",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: false,
			wantName:         "service_latencies",
			wantDescription:  "0.95th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(latency_bucket{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,le))`,
		},
		{
			name:             "group by service and operation should be reflected in name/description and query group-by",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			wantName:         "service_operation_latencies",
			wantDescription:  "0.95th quantile latency, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(latency_bucket{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,operation,le))`,
		},
		{
			name:             "two services and span kinds result in regex 'or' symbol in query",
			serviceNames:     []string{"frontend", "emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER", "SPAN_KIND_CLIENT"},
			groupByOperation: false,
			wantName:         "service_latencies",
			wantDescription:  "0.95th quantile latency, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(latency_bucket{service_name =~ "frontend|emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name,le))`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricsstore.LatenciesQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(tc),
				Quantile:            0.95,
			}
			reader, mockPrometheus := prepareMetricsReaderAndServer(t, tc.wantPromQlQuery, nil)
			defer mockPrometheus.Close()

			m, err := reader.GetLatencies(context.Background(), &params)
			require.NoError(t, err)
			assertMetrics(t, m, tc.wantLabels, tc.wantName, tc.wantDescription)
		})
	}
}

func TestGetCallRates(t *testing.T) {
	for _, tc := range []metricsTestCase{
		{
			name:             "group by service only should be reflected in name/description and query group-by",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: false,
			wantName:         "service_call_rate",
			wantDescription:  "calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`,
		},
		{
			name:             "group by service and operation should be reflected in name/description and query group-by",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			wantName:         "service_operation_call_rate",
			wantDescription:  "calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,operation)`,
		},
		{
			name:             "two services and span kinds result in regex 'or' symbol in query",
			serviceNames:     []string{"frontend", "emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER", "SPAN_KIND_CLIENT"},
			groupByOperation: false,
			wantName:         "service_call_rate",
			wantDescription:  "calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "frontend|emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricsstore.CallRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(tc),
			}
			reader, mockPrometheus := prepareMetricsReaderAndServer(t, tc.wantPromQlQuery, nil)
			defer mockPrometheus.Close()

			m, err := reader.GetCallRates(context.Background(), &params)
			require.NoError(t, err)
			assertMetrics(t, m, tc.wantLabels, tc.wantName, tc.wantDescription)
		})
	}
}

func TestGetErrorRates(t *testing.T) {
	for _, tc := range []metricsTestCase{
		{
			name:             "group by service only should be reflected in name/description and query group-by",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: false,
			wantName:         "service_error_rate",
			wantDescription:  "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name) / ` +
				`sum(rate(calls_total{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`,
		},
		{
			name:             "group by service and operation should be reflected in name/description and query group-by",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			wantName:         "service_operation_error_rate",
			wantDescription:  "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,operation) / ` +
				`sum(rate(calls_total{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,operation)`,
		},
		{
			name:             "two services and span kinds result in regex 'or' symbol in query",
			serviceNames:     []string{"frontend", "emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER", "SPAN_KIND_CLIENT"},
			groupByOperation: false,
			wantName:         "service_error_rate",
			wantDescription:  "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "frontend|emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name) / ` +
				`sum(rate(calls_total{service_name =~ "frontend|emailservice", span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricsstore.ErrorRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(tc),
			}
			reader, mockPrometheus := prepareMetricsReaderAndServer(t, tc.wantPromQlQuery, nil)
			defer mockPrometheus.Close()

			m, err := reader.GetErrorRates(context.Background(), &params)
			require.NoError(t, err)
			assertMetrics(t, m, tc.wantLabels, tc.wantName, tc.wantDescription)
		})
	}
}

func TestWarningResponse(t *testing.T) {
	params := metricsstore.ErrorRateQueryParameters{
		BaseQueryParameters: buildTestBaseQueryParametersFrom(metricsTestCase{serviceNames: []string{"foo"}}),
	}
	reader, mockPrometheus := prepareMetricsReaderAndServer(t, "", []string{"warning0", "warning1"})
	defer mockPrometheus.Close()

	m, err := reader.GetErrorRates(context.Background(), &params)
	require.NoError(t, err)
	assert.NotNil(t, m)
}

func TestGetRoundTripper(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tlsEnabled bool
	}{
		{"tls tlsEnabled", true},
		{"tls disabled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := zap.NewNop()
			rt, err := getHTTPRoundTripper(&config.Configuration{
				ServerURL:      "https://localhost:1234",
				ConnectTimeout: 9 * time.Millisecond,
				TLS: tlscfg.Options{
					Enabled: tc.tlsEnabled,
				},
			}, logger)
			require.NoError(t, err)

			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, "Bearer foo", r.Header.Get("Authorization"))
					},
				),
			)
			defer server.Close()

			req, err := http.NewRequestWithContext(
				bearertoken.ContextWithBearerToken(context.Background(), "foo"),
				http.MethodGet,
				server.URL,
				nil,
			)
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestInvalidCertFile(t *testing.T) {
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, config.Configuration{
		ServerURL:      "https://localhost:1234",
		ConnectTimeout: defaultTimeout,
		TLS: tlscfg.Options{
			Enabled: true,
			CAPath:  "foo",
		},
	})
	require.Error(t, err)
	assert.Nil(t, reader)
}

func startMockPrometheusServer(t *testing.T, wantPromQlQuery string, wantWarnings []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(wantWarnings) > 0 {
			sendResponse(t, w, "testdata/warning_response.json")
			return
		}

		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		u, err := url.Parse("http://" + r.Host + r.RequestURI + "?" + string(body))
		require.NoError(t, err)

		q := u.Query()
		promQuery := q.Get("query")
		assert.Equal(t, wantPromQlQuery, promQuery)

		mockResponsePayloadFile := "testdata/service_datapoint_response.json"
		if strings.Contains(promQuery, "by (service_name,operation") {
			mockResponsePayloadFile = "testdata/service_operation_datapoint_response.json"
		}
		sendResponse(t, w, mockResponsePayloadFile)
	}))
}

func sendResponse(t *testing.T, w http.ResponseWriter, responseFile string) error {
	bytes, err := os.ReadFile(responseFile)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(bytes))
	return err
}

func buildTestBaseQueryParametersFrom(tc metricsTestCase) metricsstore.BaseQueryParameters {
	endTime := time.Now()
	lookback := time.Minute
	step := time.Millisecond
	ratePer := 10 * time.Minute

	return metricsstore.BaseQueryParameters{
		ServiceNames:     tc.serviceNames,
		GroupByOperation: tc.groupByOperation,
		EndTime:          &endTime,
		Lookback:         &lookback,
		Step:             &step,
		RatePer:          &ratePer,
		SpanKinds:        tc.spanKinds,
	}
}

func prepareMetricsReaderAndServer(t *testing.T, wantPromQlQuery string, wantWarnings []string) (metricsstore.Reader, *httptest.Server) {
	mockPrometheus := startMockPrometheusServer(t, wantPromQlQuery, wantWarnings)

	logger := zap.NewNop()
	address := mockPrometheus.Listener.Addr().String()
	reader, err := NewMetricsReader(logger, config.Configuration{
		ServerURL:      "http://" + address,
		ConnectTimeout: defaultTimeout,
	})
	require.NoError(t, err)
	return reader, mockPrometheus
}

func assertMetrics(t *testing.T, gotMetrics *metrics.MetricFamily, wantLabels map[string]string, wantName, wantDescription string) {
	assert.Len(t, gotMetrics.Metrics, 1)
	assert.Equal(t, wantName, gotMetrics.Name)
	assert.Equal(t, wantDescription, gotMetrics.Help)
	mps := gotMetrics.Metrics[0].MetricPoints
	assert.Len(t, mps, 1)

	// There is no guaranteed order of labels, so we need to take the approach of using a map of expected values.
	labels := gotMetrics.Metrics[0].Labels
	assert.Equal(t, len(wantLabels), len(labels))
	for _, l := range labels {
		assert.Contains(t, wantLabels, l.Name)
		assert.Equal(t, wantLabels[l.Name], l.Value)
		delete(wantLabels, l.Name)
	}
	assert.Empty(t, wantLabels)
	assert.Equal(t, int64(1620351786), mps[0].Timestamp.GetSeconds())

	actualVal := mps[0].Value.(*metrics.MetricPoint_GaugeValue).GaugeValue.Value.(*metrics.GaugeValue_DoubleValue).DoubleValue
	assert.Equal(t, float64(9223372036854), actualVal)
}
