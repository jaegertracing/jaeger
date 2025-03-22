// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/prometheus/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

type (
	metricsTestCase struct {
		name             string
		serviceNames     []string
		spanKinds        []string
		groupByOperation bool
		updateConfig     func(config.Configuration) config.Configuration
		wantName         string
		wantDescription  string
		wantLabels       map[string]string
		wantPromQlQuery  string
	}
)

const defaultTimeout = 30 * time.Second

// defaultConfig should consist of the default values for the prometheus.query.* command line options.
var defaultConfig = config.Configuration{
	MetricNamespace: "",
	LatencyUnit:     "ms",
}

func tracerProvider(t *testing.T) (trace.TracerProvider, *tracetest.InMemoryExporter, func()) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	closer := func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	}
	return tp, exporter, closer
}

func TestNewMetricsReaderValidAddress(t *testing.T) {
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()
	reader, err := NewMetricsReader(config.Configuration{
		ServerURL:      "http://localhost:1234",
		ConnectTimeout: defaultTimeout,
	}, logger, tracer)
	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestNewMetricsReaderInvalidAddress(t *testing.T) {
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()
	reader, err := NewMetricsReader(config.Configuration{
		ServerURL:      "\n",
		ConnectTimeout: defaultTimeout,
	}, logger, tracer)
	require.ErrorContains(t, err, "failed to initialize prometheus client")
	assert.Nil(t, reader)
}

func TestGetMinStepDuration(t *testing.T) {
	params := metricstore.MinStepDurationQueryParameters{}
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()
	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)

	reader, err := NewMetricsReader(config.Configuration{
		ServerURL:      "http://" + listener.Addr().String(),
		ConnectTimeout: defaultTimeout,
	}, logger, tracer)
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

	params := metricstore.CallRateQueryParameters{
		BaseQueryParameters: metricstore.BaseQueryParameters{
			EndTime:  &endTime,
			Lookback: &lookback,
			Step:     &step,
			RatePer:  &ratePer,
		},
	}

	mockPrometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer mockPrometheus.Close()

	logger := zap.NewNop()
	tracer, exp, closer := tracerProvider(t)
	defer closer()
	address := mockPrometheus.Listener.Addr().String()
	reader, err := NewMetricsReader(config.Configuration{
		ServerURL:      "http://" + address,
		ConnectTimeout: defaultTimeout,
	}, logger, tracer)
	require.NoError(t, err)
	m, err := reader.GetCallRates(context.Background(), &params)
	assert.NotNil(t, m)
	require.ErrorContains(t, err, "failed executing metrics query")
	require.Len(t, exp.GetSpans(), 1, "HTTP request was traced and span reported")
	assert.Equal(t, codes.Error, exp.GetSpans()[0].Status.Code)
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
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(duration_bucket{service_name =~ "emailservice", ` +
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
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(duration_bucket{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name,le))`,
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
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(duration_bucket{service_name =~ "frontend|emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name,le))`,
		},
		{
			name:             "enable support for spanmetrics connector with a namespace",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.MetricNamespace = "span_metrics"
				cfg.LatencyUnit = "s"
				return cfg
			},
			wantName:        "service_operation_latencies",
			wantDescription: "0.95th quantile latency, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(span_metrics_duration_bucket{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name,le))`,
		},
		{
			name:             "enable support for spanmetrics connector with normalized metric name",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.NormalizeDuration = true
				cfg.LatencyUnit = "s"
				return cfg
			},
			wantName:        "service_operation_latencies",
			wantDescription: "0.95th quantile latency, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `histogram_quantile(0.95, sum(rate(duration_seconds_bucket{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name,le))`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricstore.LatenciesQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(tc),
				Quantile:            0.95,
			}
			tracer, exp, closer := tracerProvider(t)
			defer closer()
			cfg := defaultConfig
			if tc.updateConfig != nil {
				cfg = tc.updateConfig(cfg)
			}
			reader, mockPrometheus := prepareMetricsReaderAndServer(t, cfg, tc.wantPromQlQuery, nil, tracer)
			defer mockPrometheus.Close()

			m, err := reader.GetLatencies(context.Background(), &params)
			require.NoError(t, err)
			assertMetrics(t, m, tc.wantLabels, tc.wantName, tc.wantDescription)
			assert.Len(t, exp.GetSpans(), 1, "HTTP request was traced and span reported")
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
			wantPromQlQuery: `sum(rate(calls{service_name =~ "emailservice", ` +
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
			wantPromQlQuery: `sum(rate(calls{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name)`,
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
			wantPromQlQuery: `sum(rate(calls{service_name =~ "frontend|emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name)`,
		},
		{
			name:             "enable support for spanmetrics connector with a namespace",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.MetricNamespace = "span_metrics"
				return cfg
			},
			wantName:        "service_operation_call_rate",
			wantDescription: "calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(span_metrics_calls{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name)`,
		},
		{
			name:             "enable support for spanmetrics connector with normalized metric name",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.NormalizeCalls = true
				return cfg
			},
			wantName:        "service_operation_call_rate",
			wantDescription: "calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "emailservice", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricstore.CallRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(tc),
			}
			tracer, exp, closer := tracerProvider(t)
			defer closer()
			cfg := defaultConfig
			if tc.updateConfig != nil {
				cfg = tc.updateConfig(cfg)
			}
			reader, mockPrometheus := prepareMetricsReaderAndServer(t, cfg, tc.wantPromQlQuery, nil, tracer)
			defer mockPrometheus.Close()

			m, err := reader.GetCallRates(context.Background(), &params)
			require.NoError(t, err)
			assertMetrics(t, m, tc.wantLabels, tc.wantName, tc.wantDescription)
			assert.Len(t, exp.GetSpans(), 1, "HTTP request was traced and span reported")
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
			wantPromQlQuery: `sum(rate(calls{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name) / ` +
				`sum(rate(calls{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`,
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
			wantPromQlQuery: `sum(rate(calls{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name) / ` +
				`sum(rate(calls{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name)`,
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
			wantPromQlQuery: `sum(rate(calls{service_name =~ "frontend|emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name) / ` +
				`sum(rate(calls{service_name =~ "frontend|emailservice", span_kind =~ "SPAN_KIND_SERVER|SPAN_KIND_CLIENT"}[10m])) by (service_name)`,
		},
		{
			name:             "neither metric namespace nor enabling normalized metric names have an impact when spanmetrics connector is not supported",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: false,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.MetricNamespace = "span_metrics"
				cfg.NormalizeCalls = true
				return cfg
			},
			wantName:        "service_error_rate",
			wantDescription: "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
			wantLabels: map[string]string{
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(span_metrics_calls_total{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name) / ` +
				`sum(rate(span_metrics_calls_total{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`,
		},
		{
			name:             "enable support for spanmetrics connector with a metric namespace",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.MetricNamespace = "span_metrics"
				return cfg
			},
			wantName:        "service_operation_error_rate",
			wantDescription: "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(span_metrics_calls{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name) / ` +
				`sum(rate(span_metrics_calls{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name)`,
		},
		{
			name:             "enable support for spanmetrics connector with normalized metric name",
			serviceNames:     []string{"emailservice"},
			spanKinds:        []string{"SPAN_KIND_SERVER"},
			groupByOperation: true,
			updateConfig: func(cfg config.Configuration) config.Configuration {
				cfg.NormalizeCalls = true
				return cfg
			},
			wantName:        "service_operation_error_rate",
			wantDescription: "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service & operation",
			wantLabels: map[string]string{
				"operation":    "/OrderResult",
				"service_name": "emailservice",
			},
			wantPromQlQuery: `sum(rate(calls_total{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
				`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name) / ` +
				`sum(rate(calls_total{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name,span_name)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricstore.ErrorRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(tc),
			}
			tracer, exp, closer := tracerProvider(t)
			defer closer()
			cfg := defaultConfig
			if tc.updateConfig != nil {
				cfg = tc.updateConfig(cfg)
			}
			reader, mockPrometheus := prepareMetricsReaderAndServer(t, cfg, tc.wantPromQlQuery, nil, tracer)
			defer mockPrometheus.Close()

			m, err := reader.GetErrorRates(context.Background(), &params)
			require.NoError(t, err)
			assertMetrics(t, m, tc.wantLabels, tc.wantName, tc.wantDescription)
			assert.Len(t, exp.GetSpans(), 1, "HTTP request was traced and span reported")
		})
	}
}

func TestGetErrorRatesZero(t *testing.T) {
	params := metricstore.ErrorRateQueryParameters{
		BaseQueryParameters: buildTestBaseQueryParametersFrom(metricsTestCase{
			serviceNames: []string{"emailservice"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
		}),
	}
	tracer, exp, closer := tracerProvider(t)
	defer closer()

	const (
		queryErrorRate = `sum(rate(calls{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
			`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name) / ` +
			`sum(rate(calls{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`
		queryCallRate = `sum(rate(calls{service_name =~ "emailservice", ` +
			`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`
	)
	wantPromQLQueries := []string{queryErrorRate, queryCallRate}
	responses := []string{"testdata/empty_response.json", "testdata/service_datapoint_response.json"}
	var callCount int

	mockPrometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		u, err := url.Parse("http://" + r.Host + r.RequestURI + "?" + string(body))
		if assert.NoError(t, err) {
			q := u.Query()
			promQuery := q.Get("query")
			assert.Equal(t, wantPromQLQueries[callCount], promQuery)
			sendResponse(t, w, responses[callCount])
			callCount++
		}
	}))

	logger := zap.NewNop()
	address := mockPrometheus.Listener.Addr().String()

	cfg := defaultConfig
	cfg.ServerURL = "http://" + address
	cfg.ConnectTimeout = defaultTimeout

	reader, err := NewMetricsReader(cfg, logger, tracer)
	require.NoError(t, err)

	defer mockPrometheus.Close()

	m, err := reader.GetErrorRates(context.Background(), &params)
	require.NoError(t, err)

	require.Len(t, m.Metrics, 1)
	mps := m.Metrics[0].MetricPoints

	require.Len(t, mps, 1)

	// Assert that we essentially zeroed the call rate data point.
	// That is, the timestamp is the same as the call rate's data point, but the value is 0.
	actualVal := mps[0].Value.(*metrics.MetricPoint_GaugeValue).GaugeValue.Value.(*metrics.GaugeValue_DoubleValue).DoubleValue
	assert.Zero(t, actualVal)
	assert.Equal(t, int64(1620351786), mps[0].Timestamp.GetSeconds())
	assert.Len(t, exp.GetSpans(), 2, "expected an error rate query and a call rate query to be made")
}

func TestGetErrorRatesNull(t *testing.T) {
	params := metricstore.ErrorRateQueryParameters{
		BaseQueryParameters: buildTestBaseQueryParametersFrom(metricsTestCase{
			serviceNames: []string{"emailservice"},
			spanKinds:    []string{"SPAN_KIND_SERVER"},
		}),
	}
	tracer, exp, closer := tracerProvider(t)
	defer closer()

	const (
		queryErrorRate = `sum(rate(calls{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
			`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name) / ` +
			`sum(rate(calls{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`
		queryCallRate = `sum(rate(calls{service_name =~ "emailservice", ` +
			`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`
	)
	wantPromQLQueries := []string{queryErrorRate, queryCallRate}
	responses := []string{"testdata/empty_response.json", "testdata/empty_response.json"}
	var callCount int

	mockPrometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		u, err := url.Parse("http://" + r.Host + r.RequestURI + "?" + string(body))
		assert.NoError(t, err)

		q := u.Query()
		promQuery := q.Get("query")
		assert.Equal(t, wantPromQLQueries[callCount], promQuery)
		sendResponse(t, w, responses[callCount])
		callCount++
	}))

	logger := zap.NewNop()
	address := mockPrometheus.Listener.Addr().String()

	cfg := defaultConfig
	cfg.ServerURL = "http://" + address
	cfg.ConnectTimeout = defaultTimeout

	reader, err := NewMetricsReader(cfg, logger, tracer)
	require.NoError(t, err)

	defer mockPrometheus.Close()

	m, err := reader.GetErrorRates(context.Background(), &params)
	require.NoError(t, err)
	assert.Empty(t, m.Metrics, "expect no error data available")
	assert.Len(t, exp.GetSpans(), 2, "expected an error rate query and a call rate query to be made")
}

func TestGetErrorRatesErrors(t *testing.T) {
	for _, tc := range []struct {
		name               string
		failErrorRateQuery bool
		failCallRateQuery  bool
		wantErr            string
	}{
		{
			name:               "error rate query failure",
			failErrorRateQuery: true,
			wantErr:            "failed getting error metrics: failed executing metrics query: server_error: server error: 500",
		},
		{
			name:              "call rate query failure",
			failCallRateQuery: true,
			wantErr:           "failed getting call metrics: failed executing metrics query: server_error: server error: 500",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := metricstore.ErrorRateQueryParameters{
				BaseQueryParameters: buildTestBaseQueryParametersFrom(metricsTestCase{
					serviceNames: []string{"emailservice"},
					spanKinds:    []string{"SPAN_KIND_SERVER"},
				}),
			}
			tracer, _, closer := tracerProvider(t)
			defer closer()

			const (
				queryErrorRate = `sum(rate(calls{service_name =~ "emailservice", status_code = "STATUS_CODE_ERROR", ` +
					`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name) / ` +
					`sum(rate(calls{service_name =~ "emailservice", span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`
				queryCallRate = `sum(rate(calls{service_name =~ "emailservice", ` +
					`span_kind =~ "SPAN_KIND_SERVER"}[10m])) by (service_name)`
			)
			wantPromQLQueries := []string{queryErrorRate, queryCallRate}
			responses := []string{"testdata/empty_response.json", "testdata/service_datapoint_response.json"}
			var callCount int

			mockPrometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				defer r.Body.Close()

				u, err := url.Parse("http://" + r.Host + r.RequestURI + "?" + string(body))
				assert.NoError(t, err)

				q := u.Query()
				promQuery := q.Get("query")
				assert.Equal(t, wantPromQLQueries[callCount], promQuery)

				switch promQuery {
				case queryErrorRate:
					if tc.failErrorRateQuery {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						sendResponse(t, w, responses[callCount])
					}
				case queryCallRate:
					if tc.failCallRateQuery {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						sendResponse(t, w, responses[callCount])
					}
				}
				callCount++
			}))

			logger := zap.NewNop()
			address := mockPrometheus.Listener.Addr().String()

			cfg := defaultConfig
			cfg.ServerURL = "http://" + address
			cfg.ConnectTimeout = defaultTimeout

			reader, err := NewMetricsReader(cfg, logger, tracer)
			require.NoError(t, err)

			defer mockPrometheus.Close()

			_, err = reader.GetErrorRates(context.Background(), &params)
			require.Error(t, err)
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestInvalidLatencyUnit(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected a panic due to invalid latency unit")
		}
	}()
	tracer, _, closer := tracerProvider(t)
	defer closer()
	cfg := config.Configuration{
		NormalizeDuration: true,
		LatencyUnit:       "something invalid",
	}
	_, _ = NewMetricsReader(cfg, zap.NewNop(), tracer)
}

func TestWarningResponse(t *testing.T) {
	params := metricstore.ErrorRateQueryParameters{
		BaseQueryParameters: buildTestBaseQueryParametersFrom(metricsTestCase{serviceNames: []string{"foo"}}),
	}
	tracer, exp, closer := tracerProvider(t)
	defer closer()
	reader, mockPrometheus := prepareMetricsReaderAndServer(t, config.Configuration{}, "", []string{"warning0", "warning1"}, tracer)
	defer mockPrometheus.Close()

	m, err := reader.GetErrorRates(context.Background(), &params)
	require.NoError(t, err)
	assert.NotNil(t, m)
	assert.Len(t, exp.GetSpans(), 2, "expected an error rate query and a call rate query to be made")
}

type fakePromServer struct {
	*httptest.Server
	authReceived atomic.Pointer[string]
}

func newFakePromServer(t *testing.T) *fakePromServer {
	s := &fakePromServer{}
	s.Server = httptest.NewServer(
		http.HandlerFunc(
			func(_ http.ResponseWriter, r *http.Request) {
				t.Logf("Request to fake Prometheus server %+v", r)
				h := r.Header.Get("Authorization")
				s.authReceived.Store(&h)
			},
		),
	)
	return s
}

func (s *fakePromServer) getAuth() string {
	return *s.authReceived.Load()
}

func TestGetRoundTripperTLSConfig(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tlsEnabled bool
	}{
		{"tls tlsEnabled", true},
		{"tls disabled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &config.Configuration{
				ConnectTimeout:           9 * time.Millisecond,
				TLS:                      configtls.ClientConfig{},
				TokenOverrideFromContext: true,
			}
			rt, err := getHTTPRoundTripper(config)
			require.NoError(t, err)

			server := newFakePromServer(t)
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
			assert.Equal(t, "Bearer foo", server.getAuth())
		})
	}
}

func TestGetRoundTripperTokenFile(t *testing.T) {
	const wantBearer = "token from file"

	file, err := os.CreateTemp("", "token_")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(file.Name())) }()

	_, err = file.Write([]byte(wantBearer))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	rt, err := getHTTPRoundTripper(&config.Configuration{
		ConnectTimeout:           time.Second,
		TokenFilePath:            file.Name(),
		TokenOverrideFromContext: false,
	})
	require.NoError(t, err)

	server := newFakePromServer(t)
	defer server.Close()

	ctx := bearertoken.ContextWithBearerToken(context.Background(), "tokenFromRequest")
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		server.URL,
		nil,
	)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Bearer "+wantBearer, server.getAuth())
}

func TestGetRoundTripperTokenFromContext(t *testing.T) {
	file, err := os.CreateTemp("", "token_")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(file.Name())) }()
	_, err = file.Write([]byte("token from file"))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	rt, err := getHTTPRoundTripper(&config.Configuration{
		ConnectTimeout:           time.Second,
		TokenFilePath:            file.Name(),
		TokenOverrideFromContext: true,
	})
	require.NoError(t, err)

	server := newFakePromServer(t)
	defer server.Close()

	ctx := bearertoken.ContextWithBearerToken(context.Background(), "tokenFromRequest")
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		server.URL,
		nil,
	)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Bearer tokenFromRequest", server.getAuth())
}

func TestGetRoundTripperTokenError(t *testing.T) {
	tokenFilePath := "this file does not exist"

	_, err := getHTTPRoundTripper(&config.Configuration{
		TokenFilePath: tokenFilePath,
	})
	assert.ErrorContains(t, err, "failed to get token from file")
}

func TestInvalidCertFile(t *testing.T) {
	logger := zap.NewNop()
	tracer, _, closer := tracerProvider(t)
	defer closer()
	reader, err := NewMetricsReader(config.Configuration{
		ServerURL:      "https://localhost:1234",
		ConnectTimeout: defaultTimeout,
		TLS: configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: "foo",
			},
		},
	}, logger, tracer)
	require.Error(t, err)
	assert.Nil(t, reader)
}

func TestCreatePromClientWithExtraQueryParameters(t *testing.T) {
	extraParams := map[string]string{
		"param1": "value1",
		"param2": "value2",
	}

	cfg := config.Configuration{
		ServerURL:        "http://localhost:1234?param1=value0",
		ExtraQueryParams: extraParams,
	}

	expParams := map[string][]string{
		"param1": {"value0", "value1"},
		"param2": {"value2"},
	}

	customClient, err := createPromClient(cfg)
	require.NoError(t, err)

	u := customClient.URL("", nil)

	q := u.Query()

	for k, v := range expParams {
		sort.Strings(q[k])
		require.Equal(t, v, q[k])
	}
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
		assert.NoError(t, err)

		q := u.Query()
		promQuery := q.Get("query")
		assert.Equal(t, wantPromQlQuery, promQuery)

		mockResponsePayloadFile := "testdata/service_datapoint_response.json"
		if strings.Contains(promQuery, "by (service_name,span_name") {
			mockResponsePayloadFile = "testdata/service_span_name_datapoint_response.json"
		}
		sendResponse(t, w, mockResponsePayloadFile)
	}))
}

func sendResponse(t *testing.T, w http.ResponseWriter, responseFile string) {
	bytes, err := os.ReadFile(responseFile)
	require.NoError(t, err)

	_, err = w.Write(bytes)
	require.NoError(t, err)
}

func buildTestBaseQueryParametersFrom(tc metricsTestCase) metricstore.BaseQueryParameters {
	endTime := time.Now()
	lookback := time.Minute
	step := time.Millisecond
	ratePer := 10 * time.Minute

	return metricstore.BaseQueryParameters{
		ServiceNames:     tc.serviceNames,
		GroupByOperation: tc.groupByOperation,
		EndTime:          &endTime,
		Lookback:         &lookback,
		Step:             &step,
		RatePer:          &ratePer,
		SpanKinds:        tc.spanKinds,
	}
}

func prepareMetricsReaderAndServer(t *testing.T, cfg config.Configuration, wantPromQlQuery string, wantWarnings []string, tracer trace.TracerProvider) (metricstore.Reader, *httptest.Server) {
	mockPrometheus := startMockPrometheusServer(t, wantPromQlQuery, wantWarnings)

	logger := zap.NewNop()
	address := mockPrometheus.Listener.Addr().String()

	cfg.ServerURL = "http://" + address
	cfg.ConnectTimeout = defaultTimeout

	reader, err := NewMetricsReader(cfg, logger, tracer)
	require.NoError(t, err)

	return reader, mockPrometheus
}

func assertMetrics(t *testing.T, gotMetrics *metrics.MetricFamily, wantLabels map[string]string, wantName, wantDescription string) {
	assert.Len(t, gotMetrics.Metrics, 1)
	assert.Equal(t, wantName, gotMetrics.Name)
	assert.Equal(t, wantDescription, gotMetrics.Help)
	mps := gotMetrics.Metrics[0].MetricPoints
	assert.Len(t, mps, 1)

	// logging for expected and actual labels
	t.Logf("Expected labels: %v\n", wantLabels)
	t.Logf("Actual labels: %v\n", gotMetrics.Metrics[0].Labels)

	// There is no guaranteed order of labels, so we need to take the approach of using a map of expected values.
	labels := gotMetrics.Metrics[0].Labels
	assert.Equal(t, len(wantLabels), len(labels))
	for _, l := range labels {
		assert.Contains(t, wantLabels, l.Name)
		assert.Equal(t, wantLabels[l.Name], l.Value)
		delete(wantLabels, l.Name)
	}
	assert.Empty(t, wantLabels)

	// Additional logging to show that all expected labels were found and matched
	t.Logf("Remaining expected labels after matching: %v\n", wantLabels)
	t.Logf("\n")

	assert.Equal(t, int64(1620351786), mps[0].Timestamp.GetSeconds())

	actualVal := mps[0].Value.(*metrics.MetricPoint_GaugeValue).GaugeValue.Value.(*metrics.GaugeValue_DoubleValue).DoubleValue
	assert.InDelta(t, float64(9223372036854), actualVal, 0.01)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
