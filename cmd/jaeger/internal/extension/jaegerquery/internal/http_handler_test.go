// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	deepdependencies "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/ddg"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/qualitymetrics"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	metricsmocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/mocks"
	depsmocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	ui "github.com/jaegertracing/jaeger/internal/uimodel"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	errStorageMsg = "storage error"
	errStorage    = errors.New(errStorageMsg)

	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}

	mockTraceID = model.NewTraceID(0, 123456)
	startTime   = time.Date(2020, time.January, 1, 13, 0, 0, 0, time.UTC)
	endTime     = time.Date(2020, time.January, 1, 14, 0, 0, 0, time.UTC)
	mockTrace   = &model.Trace{
		Spans: []*model.Span{
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(1),
				Process: &model.Process{ServiceName: "service"},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(2),
				Process: &model.Process{ServiceName: "service"},
			},
		},
		Warnings: []string{},
	}
)

// Iterator helper functions for cleaner test mocks
func emptyIter() iter.Seq2[[]ptrace.Traces, error] {
	return func(func([]ptrace.Traces, error) bool) {}
}

func tracesIter(traces ...ptrace.Traces) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		yield(traces, nil)
	}
}

// structuredTraceResponse is similar to structuredResponse but defines `data`
// explicitly as []*ui.Trace, making it easier to parse & validate.
type structuredTraceResponse struct {
	Traces []*ui.Trace       `json:"data"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Errors []structuredError `json:"errors"`
}

func initializeTestServerWithHandler(t *testing.T, queryOptions querysvc.QueryServiceOptions, options ...HandlerOption) *testServer {
	return initializeTestServerWithOptions(
		t,
		&tenancy.Manager{},
		queryOptions,
		append(
			[]HandlerOption{
				HandlerOptions.Logger(zap.NewNop()),
				// add options for test coverage
				HandlerOptions.Prefix(defaultAPIPrefix),
				HandlerOptions.BasePath("/"),
			},
			options...,
		)...,
	)
}

func initializeTestServerWithOptions(
	t *testing.T,
	tenancyMgr *tenancy.Manager,
	queryOptions querysvc.QueryServiceOptions,
	options ...HandlerOption,
) *testServer {
	options = append(options, HandlerOptions.Logger(zaptest.NewLogger(t)))
	traceReader := &tracestoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	qs := querysvc.NewQueryService(traceReader, dependencyStorage, queryOptions)
	r := http.NewServeMux()
	apiHandler := NewAPIHandler(qs, options...)
	apiHandler.RegisterRoutes(r)
	ts := &testServer{
		server:           httptest.NewServer(tenancy.ExtractTenantHTTPHandler(tenancyMgr, r)),
		traceReader:      traceReader,
		dependencyReader: dependencyStorage,
		handler:          apiHandler,
	}
	t.Cleanup(func() {
		ts.server.Close()
	})
	return ts
}

func initializeTestServer(t *testing.T, options ...HandlerOption) *testServer {
	return initializeTestServerWithHandler(t, querysvc.QueryServiceOptions{}, options...)
}

type testServer struct {
	traceReader      *tracestoremocks.Reader
	dependencyReader *depsmocks.Reader
	handler          *APIHandler
	server           *httptest.Server
}

func withTestServer(t *testing.T, doTest func(s *testServer), queryOptions querysvc.QueryServiceOptions, options ...HandlerOption) {
	ts := initializeTestServerWithOptions(t, &tenancy.Manager{}, queryOptions, options...)
	doTest(ts)
}

func TestLogOnServerError(t *testing.T) {
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	traceReader := &tracestoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	qs := querysvc.NewQueryService(traceReader, dependencyStorage, querysvc.QueryServiceOptions{})
	h := NewAPIHandler(qs, HandlerOptions.Logger(logger))
	e := errors.New("test error")
	h.handleError(&httptest.ResponseRecorder{}, e, http.StatusInternalServerError)
	require.Len(t, logs.All(), 1)
	log := logs.All()[0]
	assert.Equal(t, "HTTP handler, Internal Server Error", log.Message)
	require.Len(t, log.Context, 1)
	assert.Equal(t, e, log.Context[0].Interface)
}

// httpResponseErrWriter implements the http.ResponseWriter interface that returns an error on Write.
type httpResponseErrWriter struct{}

func (*httpResponseErrWriter) Write([]byte) (int, error) {
	return 0, errors.New("failed to write")
}
func (*httpResponseErrWriter) WriteHeader(int /* statusCode */) {}
func (*httpResponseErrWriter) Header() http.Header {
	return http.Header{}
}

func TestWriteJSON(t *testing.T) {
	testCases := []struct {
		name         string
		data         any
		param        string
		output       string
		httpWriteErr bool
	}{
		{
			name:   "no pretty print param passed",
			data:   struct{ Data string }{Data: "Bender"},
			output: `{"Data":"Bender"}`,
		},
		{
			name:   "pretty print explicitly disabled",
			data:   struct{ Data string }{Data: "Bender"},
			param:  "?prettyPrint=false",
			output: `{"Data":"Bender"}`,
		},
		{
			name:   "pretty print enabled",
			data:   struct{ Data string }{Data: "Bender"},
			param:  "?prettyPrint=x",
			output: "{\n    \"Data\": \"Bender\"\n}",
		},
		{
			name:   "fail JSON marshal",
			data:   struct{ Data float64 }{Data: math.Inf(1)},
			output: `failed marshalling HTTP response to JSON: json: unsupported value: +Inf`,
		},
		{
			name:         "fail http write",
			data:         struct{ Data string }{Data: "Bender"},
			httpWriteErr: true,
		},
	}

	get := func(url string) string {
		res, err := http.Get(url)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return string(body)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			apiHandler := &APIHandler{
				logger: zap.NewNop(),
			}
			httpHandler := func(w http.ResponseWriter, r *http.Request) {
				if testCase.httpWriteErr {
					w = &httpResponseErrWriter{}
				}
				apiHandler.writeJSON(w, r, &testCase.data)
			}
			server := httptest.NewServer(http.HandlerFunc(httpHandler))
			defer server.Close()

			out := get(server.URL + testCase.param)
			assert.Contains(t, out, testCase.output)
		})
	}
}

func TestGetMetricsSuccess(t *testing.T) {
	mr := &metricsmocks.Reader{}
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.MetricsQueryService(mr),
	}
	ts := initializeTestServer(t, apiHandlerOptions...)
	expectedLabel := &metrics.Label{
		Name:  "service_name",
		Value: "emailservice",
	}
	expectedMetricPoint := &metrics.MetricPoint{
		Timestamp: &types.Timestamp{Seconds: time.Now().Unix()},
		Value: &metrics.MetricPoint_GaugeValue{
			GaugeValue: &metrics.GaugeValue{
				Value: &metrics.GaugeValue_DoubleValue{DoubleValue: 0.9},
			},
		},
	}
	expectedMetricsQueryResponse := &metrics.MetricFamily{
		Name: "the metrics",
		Type: metrics.MetricType_GAUGE,
		Metrics: []*metrics.Metric{
			{
				Labels:       []*metrics.Label{expectedLabel},
				MetricPoints: []*metrics.MetricPoint{expectedMetricPoint},
			},
		},
	}

	for _, tc := range []struct {
		name                       string
		urlPath                    string
		mockedQueryMethod          string
		mockedQueryMethodParamType string
	}{
		{
			name:                       "latencies",
			urlPath:                    "/api/metrics/latencies?service=emailservice&quantile=0.95",
			mockedQueryMethod:          "GetLatencies",
			mockedQueryMethodParamType: "*metricstore.LatenciesQueryParameters",
		},
		{
			name:                       "call rates",
			urlPath:                    "/api/metrics/calls?service=emailservice",
			mockedQueryMethod:          "GetCallRates",
			mockedQueryMethodParamType: "*metricstore.CallRateQueryParameters",
		},
		{
			name:                       "error rates",
			urlPath:                    "/api/metrics/errors?service=emailservice",
			mockedQueryMethod:          "GetErrorRates",
			mockedQueryMethodParamType: "*metricstore.ErrorRateQueryParameters",
		},
		{
			name:                       "error rates with pretty print",
			urlPath:                    "/api/metrics/errors?service=emailservice&prettyPrint=true",
			mockedQueryMethod:          "GetErrorRates",
			mockedQueryMethodParamType: "*metricstore.ErrorRateQueryParameters",
		},
		{
			name:                       "error rates with spanKinds",
			urlPath:                    "/api/metrics/errors?service=emailservice&spanKind=client",
			mockedQueryMethod:          "GetErrorRates",
			mockedQueryMethodParamType: "*metricstore.ErrorRateQueryParameters",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare
			mr.On(
				tc.mockedQueryMethod,
				mock.Anything,
				mock.AnythingOfType(tc.mockedQueryMethodParamType),
			).Return(expectedMetricsQueryResponse, nil).Once()

			// Test
			var response metrics.MetricFamily
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			require.NoError(t, err)
			assert.Equal(t, expectedMetricsQueryResponse, &response)
		})
	}
}

func TestGetQualityMetrics(t *testing.T) {
	handler := &APIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/quality-metrics", http.NoBody)
	rr := httptest.NewRecorder()
	handler.getQualityMetrics(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	var resp qualitymetrics.TQualityMetrics
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Contains(t, resp.TraceQualityDocumentationLink, "github.com/jaegertracing")
}

func TestGetDeepDependenciesData(t *testing.T) {
	handler := &APIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/deep-dependencies?service=customer", http.NoBody)
	rr := httptest.NewRecorder()

	handler.deepDependencies(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp deepdependencies.TDdgPayload
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotEmpty(t, resp.Dependencies, "Dependencies should not be empty")
}

func TestMetricsReaderError(t *testing.T) {
	metricsReader := &metricsmocks.Reader{}
	ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))

	for _, tc := range []struct {
		name                       string
		urlPath                    string
		mockedQueryMethod          string
		mockedQueryMethodParamType string
		mockedResponse             any
		wantErrorMessage           string
	}{
		{
			urlPath:                    "/api/metrics/calls?service=emailservice",
			mockedQueryMethod:          "GetCallRates",
			mockedQueryMethodParamType: "*metricstore.CallRateQueryParameters",
			mockedResponse:             nil,
			wantErrorMessage:           "error fetching call rates",
		},
	} {
		t.Run(tc.wantErrorMessage, func(t *testing.T) {
			// Prepare
			metricsReader.On(
				tc.mockedQueryMethod,
				mock.Anything,
				mock.AnythingOfType(tc.mockedQueryMethodParamType),
			).Return(tc.mockedResponse, errors.New(tc.wantErrorMessage)).Once()

			// Test
			var response metrics.MetricFamily
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			assert.ErrorContains(t, err, tc.wantErrorMessage)
		})
	}
}

func TestMetricsQueryDisabled(t *testing.T) {
	disabledReader, err := disabled.NewMetricsReader()
	require.NoError(t, err)

	ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(disabledReader))
	defer ts.server.Close()

	for _, tc := range []struct {
		name             string
		urlPath          string
		wantErrorMessage string
	}{
		{
			name:             "metrics query disabled error returned when fetching latency metrics",
			urlPath:          "/api/metrics/latencies?service=emailservice&quantile=0.95",
			wantErrorMessage: disabled.ErrDisabled.Error(),
		},
		{
			name:             "dimensions endpoint on disabled backend returns 501",
			urlPath:          "/api/metrics/dimensions",
			wantErrorMessage: disabled.ErrDisabled.Error(),
		},
		{
			name:             "filtered calls on disabled backend returns 501",
			urlPath:          "/api/metrics/calls?service=svc&filter=k8s.cluster:foo",
			wantErrorMessage: disabled.ErrDisabled.Error(),
		},
		{
			name:             "filtered latencies on disabled backend returns 501",
			urlPath:          "/api/metrics/latencies?service=svc&quantile=0.95&filter=k8s.cluster:foo",
			wantErrorMessage: disabled.ErrDisabled.Error(),
		},
		{
			name:             "filtered errors on disabled backend returns 501",
			urlPath:          "/api/metrics/errors?service=svc&filter=k8s.cluster:foo",
			wantErrorMessage: disabled.ErrDisabled.Error(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Test
			var response any
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			var httpErr *HTTPError
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, http.StatusNotImplemented, httpErr.StatusCode)
			require.Contains(t, httpErr.Body, tc.wantErrorMessage)
		})
	}
}

func TestGetDimensionsEndpoint(t *testing.T) {
	metricsReader := &metricsmocks.Reader{}
	ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))

	configured := []metricstore.Dimension{
		{Name: "deployment.environment", DisplayName: "Environment", Values: []string{"prod", "staging"}},
		{Name: "k8s.cluster"},
	}
	metricsReader.On("GetDimensions", mock.Anything).Return(configured, nil).Once()

	var got []metricstore.Dimension
	require.NoError(t, getJSON(ts.server.URL+"/api/metrics/dimensions", &got))
	assert.Equal(t, configured, got)
}

func TestGetDimensionsEndpointEmpty(t *testing.T) {
	// Backends without dimension support return nil/empty; the handler must
	// emit a JSON array (not null) so the UI can iterate without a nil-guard.
	metricsReader := &metricsmocks.Reader{}
	ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))
	metricsReader.On("GetDimensions", mock.Anything).Return(nil, nil).Once()

	resp, err := http.Get(ts.server.URL + "/api/metrics/dimensions")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(bytes.TrimSpace(body)))
}

func TestGetDimensionsEndpointError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "internal failure", err: assert.AnError, status: http.StatusInternalServerError},
		{name: "unsupported", err: fmt.Errorf("dimensions: %w", errors.ErrUnsupported), status: http.StatusNotImplemented},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &metricsmocks.Reader{}
			ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(reader))
			reader.On("GetDimensions", mock.Anything).Return(nil, tc.err).Once()
			var response any
			err := getJSON(ts.server.URL+"/api/metrics/dimensions", &response)
			var httpErr *HTTPError
			require.ErrorAs(t, err, &httpErr)
			assert.Equal(t, tc.status, httpErr.StatusCode)
			assert.Contains(t, httpErr.Body, tc.err.Error())
			reader.AssertExpectations(t)
		})
	}
}

func TestMetricsMalformedQuery(t *testing.T) {
	for _, endpoint := range []string{"calls", "errors", "latencies"} {
		for _, query := range []string{
			"filter=k8s.cluster:%ZZ",
			"filter=k8s.cluster:%",
			"filter=k8s.cluster:prod;staging",
			"filter=k8s.cluster:prod&filter=deployment.environment:%ZZ",
		} {
			t.Run(endpoint+"/"+query, func(t *testing.T) {
				reader := &metricsmocks.Reader{}
				ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(reader))
				var response any
				err := getJSON(ts.server.URL+"/api/metrics/"+endpoint+"?service=svc&quantile=0.95&"+query, &response)
				var httpErr *HTTPError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
				assert.Contains(t, httpErr.Body, "invalid query")
				assert.Empty(t, reader.Calls, "malformed queries must not reach the reader")
			})
		}
	}
}

func TestMetricFilterConfiguredOrder(t *testing.T) {
	dims := []metricstore.Dimension{{Name: "z"}, {Name: "a"}, {Name: "m"}}
	for range 20 {
		err := validateMetricFilters(map[string]string{"unknown": "value"}, dims)
		require.EqualError(t, err, `unknown filter dimension "unknown"; configured: [z a m]`)
	}
}

func TestMetricFilterValidationOrder(t *testing.T) {
	dims := []metricstore.Dimension{
		{Name: "z"},
		{Name: "a", Values: []string{"prod"}},
		{Name: "m"},
	}
	for _, tc := range []struct {
		name    string
		filters map[string]string
		wantErr string
	}{
		{
			name:    "unknown keys",
			filters: map[string]string{"y": "value", "b": "value"},
			wantErr: `unknown filter dimension "b"; configured: [z a m]`,
		},
		{
			name:    "invalid value before unknown key",
			filters: map[string]string{"unknown": "value", "a": "dev", "m": "bad\nvalue"},
			wantErr: `value "dev" not allowed for filter "a"; allowed: [prod]`,
		},
		{
			name:    "valid value before invalid filters",
			filters: map[string]string{"a": "prod", "unknown": "value", "m": "bad\nvalue"},
			wantErr: `invalid value for filter "m" (control characters, quotes, and backslashes are not allowed)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for range 20 {
				require.EqualError(t, validateMetricFilters(tc.filters, dims), tc.wantErr)
			}
		})
	}
}

func TestMetricsFilterMalformed(t *testing.T) {
	for _, filter := range []string{"missing-colon", ":value", "key:"} {
		t.Run(filter, func(t *testing.T) {
			metricsReader := &metricsmocks.Reader{}
			ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))

			var response any
			err := getJSON(ts.server.URL+"/api/metrics/calls?service=svc&filter="+filter, &response)

			var httpErr *HTTPError
			require.ErrorAs(t, err, &httpErr)
			assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
			assert.Contains(t, httpErr.Body, "malformed filter")
			metricsReader.AssertNotCalled(t, "GetDimensions", mock.Anything)
		})
	}
}

func TestMetricsFilterValidation(t *testing.T) {
	configured := []metricstore.Dimension{
		{Name: "deployment.environment", DisplayName: "Environment", Values: []string{"prod", "staging"}},
		{Name: "k8s.cluster"}, // free-text dimension
	}
	// Note: assertions match against JSON-encoded response bodies, where double
	// quotes are escaped as \". Use substrings that survive that escaping.
	for _, tc := range []struct {
		name     string
		urlPath  string
		wantCode int
		wantMsg  string
	}{
		{
			name:     "unknown filter key returns 400",
			urlPath:  "/api/metrics/calls?service=svc&filter=unknown.key:v",
			wantCode: http.StatusBadRequest,
			wantMsg:  `unknown filter dimension`,
		},
		{
			name:     "value not in declared set returns 400",
			urlPath:  "/api/metrics/calls?service=svc&filter=deployment.environment:notInSet",
			wantCode: http.StatusBadRequest,
			wantMsg:  `not allowed for filter`,
		},
		{
			name:     "free-text value with control character returns 400",
			urlPath:  "/api/metrics/calls?service=svc&filter=k8s.cluster:bad%0Avalue",
			wantCode: http.StatusBadRequest,
			wantMsg:  "invalid value for filter",
		},
		{
			name:     "free-text value with DEL returns 400",
			urlPath:  "/api/metrics/calls?service=svc&filter=k8s.cluster:bad%7Fvalue",
			wantCode: http.StatusBadRequest,
			wantMsg:  "invalid value for filter",
		},
		{
			name:     "free-text value with double-quote returns 400",
			urlPath:  `/api/metrics/calls?service=svc&filter=k8s.cluster:bad"value`,
			wantCode: http.StatusBadRequest,
			wantMsg:  `invalid value for filter`,
		},
		{
			name:     "free-text value with backslash returns 400",
			urlPath:  `/api/metrics/calls?service=svc&filter=k8s.cluster:bad\value`,
			wantCode: http.StatusBadRequest,
			wantMsg:  `invalid value for filter`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsReader := &metricsmocks.Reader{}
			ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))
			metricsReader.On("GetDimensions", mock.Anything).Return(configured, nil).Once()

			resp, err := http.Get(ts.server.URL + tc.urlPath)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tc.wantCode, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), tc.wantMsg)
			metricsReader.AssertNumberOfCalls(t, "GetDimensions", 1)
			assert.Len(t, metricsReader.Calls, 1, "invalid filters must not execute a metrics query")
		})
	}
}

func TestMetricsFilterHappyPath(t *testing.T) {
	configured := []metricstore.Dimension{
		{Name: "deployment.environment", Values: []string{"prod", "staging"}},
	}
	metricsReader := &metricsmocks.Reader{}
	ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))

	metricsReader.On("GetDimensions", mock.Anything).Return(configured, nil).Once()
	metricsReader.On(
		"GetCallRates",
		mock.Anything,
		mock.MatchedBy(func(p *metricstore.CallRateQueryParameters) bool {
			return p.Filters["deployment.environment"] == "prod"
		}),
	).Return(&metrics.MetricFamily{}, nil).Once()

	var response metrics.MetricFamily
	require.NoError(t, getJSON(
		ts.server.URL+"/api/metrics/calls?service=svc&filter=deployment.environment:prod",
		&response,
	))
}

func TestMetricsFilterGetDimensionsError(t *testing.T) {
	for _, endpoint := range []string{"calls", "latencies", "errors"} {
		t.Run(endpoint, func(t *testing.T) {
			metricsReader := &metricsmocks.Reader{}
			ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))
			metricsReader.
				On("GetDimensions", mock.Anything).
				Return(nil, assert.AnError).
				Once()

			var response any
			err := getJSON(ts.server.URL+"/api/metrics/"+endpoint+"?service=svc&quantile=0.95&filter=k8s.cluster:foo", &response)

			var httpErr *HTTPError
			require.ErrorAs(t, err, &httpErr)
			assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
			assert.Contains(t, httpErr.Body, assert.AnError.Error())
			metricsReader.AssertExpectations(t)
		})
	}
}

func TestMetricsFilterRequestOnBackendWithoutDimensions(t *testing.T) {
	// A backend that advertises no dimensions must reject filter requests
	// with 501; otherwise the operator would silently get an unfiltered
	// response when they intended to filter.
	for _, endpoint := range []string{"calls", "errors", "latencies"} {
		t.Run(endpoint, func(t *testing.T) {
			metricsReader := &metricsmocks.Reader{}
			ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))
			metricsReader.On("GetDimensions", mock.Anything).Return(nil, nil).Once()

			resp, err := http.Get(ts.server.URL + "/api/metrics/" + endpoint + "?service=svc&quantile=0.95&filter=anything:x")
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "metric filtering is not supported by this backend")
			metricsReader.AssertExpectations(t)
			assert.Len(t, metricsReader.Calls, 1, "unsupported filters must not execute a metrics query")
		})
	}
}

// getJSON fetches a JSON document from a server via HTTP GET
func getJSON(url string, out any) error {
	return getJSONCustomHeaders(url, make(map[string]string), out)
}

func getJSONCustomHeaders(url string, additionalHeaders map[string]string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	return execJSON(req, additionalHeaders, out)
}

// postJSON submits a JSON document to a server via HTTP POST and parses response as JSON.
func postJSON(url string, req any, out any) error {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(req); err != nil {
		return err
	}
	r, err := http.NewRequest(http.MethodPost, url, buf)
	if err != nil {
		return err
	}
	return execJSON(r, make(map[string]string), out)
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%d error from server: %s", e.StatusCode, e.Body)
}

// execJSON executes an http request against a server and parses response as JSON
func execJSON(req *http.Request, additionalHeaders map[string]string, out any) error {
	req.Header.Add("Accept", "application/json")
	for k, v := range additionalHeaders {
		req.Header.Add(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode > 399 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	if protoMessage, ok := out.(proto.Message); ok {
		unmarshaler := new(jsonpb.Unmarshaler)
		return unmarshaler.Unmarshal(resp.Body, protoMessage)
	}
	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(out)
}

func TestNewAPIHandler_Defaults(t *testing.T) {
	aH := NewAPIHandler(&querysvc.QueryService{})
	assert.NotNil(t, aH.logger)
	assert.NotNil(t, aH.tracer)
}
