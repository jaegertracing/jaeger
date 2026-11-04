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

func TestRemovedTraceRoutes(t *testing.T) {
	ts := initializeTestServer(t)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/traces/123456"},
		{http.MethodPost, "/api/transform"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.server.URL+tc.path, nil)
			require.NoError(t, err)
			resp, err := ts.server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
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

	const urlPath = "/api/metrics/latencies?service=emailservice&quantile=0.95"
	var response any
	err = getJSON(ts.server.URL+urlPath, &response)
	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusNotImplemented, httpErr.StatusCode)
	require.Contains(t, httpErr.Body, "currently disabled")
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

// Generates a JSON response that the server should produce given a certain error code and error.
func parsedError(code int, err string) string {
	return fmt.Sprintf(`%d error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":%d,"msg":"%s"}]}`+"\n", code, code, err)
}

func TestNewAPIHandler_Defaults(t *testing.T) {
	aH := NewAPIHandler(&querysvc.QueryService{})
	assert.NotNil(t, aH.logger)
	assert.NotNil(t, aH.tracer)
}
