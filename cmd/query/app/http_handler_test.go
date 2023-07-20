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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	testHttp "github.com/stretchr/testify/http"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	metricsmocks "github.com/jaegertracing/jaeger/storage/metricsstore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	errStorageMsg = "storage error"
	errStorage    = errors.New(errStorageMsg)
	errAdjustment = errors.New("adjustment error")

	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}

	mockTraceID = model.NewTraceID(0, 123456)
	mockTrace   = &model.Trace{
		Spans: []*model.Span{
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(1),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(2),
				Process: &model.Process{},
			},
		},
		Warnings: []string{},
	}
)

// structuredTraceResponse is similar to structuredResponse but defines `data`
// explicitly as []*ui.Trace, making it easier to parse & validate.
type structuredTraceResponse struct {
	Traces []*ui.Trace       `json:"data"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Errors []structuredError `json:"errors"`
}

func initializeTestServerWithHandler(queryOptions querysvc.QueryServiceOptions, options ...HandlerOption) *testServer {
	return initializeTestServerWithOptions(
		&tenancy.Manager{},
		queryOptions,
		append(
			[]HandlerOption{
				HandlerOptions.Logger(zap.NewNop()),
				// add options for test coverage
				HandlerOptions.Prefix(defaultAPIPrefix),
				HandlerOptions.BasePath("/"),
				HandlerOptions.QueryLookbackDuration(defaultTraceQueryLookbackDuration),
			},
			options...,
		)...,
	)
}

func initializeTestServerWithOptions(tenancyMgr *tenancy.Manager, queryOptions querysvc.QueryServiceOptions, options ...HandlerOption) *testServer {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	qs := querysvc.NewQueryService(readStorage, dependencyStorage, queryOptions)
	r := NewRouter()
	handler := NewAPIHandler(qs, tenancyMgr, options...)
	handler.RegisterRoutes(r)
	return &testServer{
		server:           httptest.NewServer(tenancy.ExtractTenantHTTPHandler(tenancyMgr, r)),
		spanReader:       readStorage,
		dependencyReader: dependencyStorage,
		handler:          handler,
	}
}

func initializeTestServer(options ...HandlerOption) *testServer {
	return initializeTestServerWithHandler(querysvc.QueryServiceOptions{}, options...)
}

type testServer struct {
	spanReader       *spanstoremocks.Reader
	dependencyReader *depsmocks.Reader
	handler          *APIHandler
	server           *httptest.Server
}

func withTestServer(doTest func(s *testServer), queryOptions querysvc.QueryServiceOptions, options ...HandlerOption) {
	ts := initializeTestServerWithOptions(&tenancy.Manager{}, queryOptions, options...)
	defer ts.server.Close()
	doTest(ts)
}

func TestGetTraceSuccess(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
}

type logData struct {
	e zapcore.Entry
	f []zapcore.Field
}

type testLogger struct {
	logs *[]logData
}

func (testLogger) Enabled(zapcore.Level) bool          { return true }
func (l testLogger) With([]zapcore.Field) zapcore.Core { return l }
func (testLogger) Sync() error                         { return nil }
func (l testLogger) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(e, l)
}

func (l testLogger) Write(e zapcore.Entry, f []zapcore.Field) error {
	*l.logs = append(*l.logs, logData{e: e, f: f})
	return nil
}

func TestLogOnServerError(t *testing.T) {
	l := &testLogger{
		logs: &[]logData{},
	}
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	qs := querysvc.NewQueryService(readStorage, dependencyStorage, querysvc.QueryServiceOptions{})
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.Logger(zap.New(l)),
	}
	h := NewAPIHandler(qs, &tenancy.Manager{}, apiHandlerOptions...)
	e := errors.New("test error")
	h.handleError(&testHttp.TestResponseWriter{}, e, http.StatusInternalServerError)
	require.Equal(t, 1, len(*l.logs))
	assert.Equal(t, "HTTP handler, Internal Server Error", (*l.logs)[0].e.Message)
	assert.Equal(t, 1, len((*l.logs)[0].f))
	assert.Equal(t, e, (*l.logs)[0].f[0].Interface)
}

// httpResponseErrWriter implements the http.ResponseWriter interface that returns an error on Write.
type httpResponseErrWriter struct{}

func (h *httpResponseErrWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("failed to write")
}
func (h *httpResponseErrWriter) WriteHeader(statusCode int) {}
func (h *httpResponseErrWriter) Header() http.Header {
	return http.Header{}
}

func TestWriteJSON(t *testing.T) {
	testCases := []struct {
		name         string
		data         interface{}
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

func TestGetTrace(t *testing.T) {
	testCases := []struct {
		suffix      string
		numSpanRefs int
	}{
		{suffix: "", numSpanRefs: 0},
		{suffix: "?raw=true", numSpanRefs: 1}, // bad span reference is not filtered out
		{suffix: "?raw=false", numSpanRefs: 0},
	}

	makeMockTrace := func(t *testing.T) *model.Trace {
		out := new(bytes.Buffer)
		err := new(jsonpb.Marshaler).Marshal(out, mockTrace)
		require.NoError(t, err)
		var trace model.Trace
		require.NoError(t, jsonpb.Unmarshal(out, &trace))
		trace.Spans[1].References = []model.SpanRef{
			{TraceID: model.NewTraceID(0, 0)},
		}
		return &trace
	}

	extractTraces := func(t *testing.T, response *structuredResponse) []ui.Trace {
		var traces []ui.Trace
		bytes, err := json.Marshal(response.Data)
		require.NoError(t, err)
		err = json.Unmarshal(bytes, &traces)
		require.NoError(t, err)
		return traces
	}

	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.suffix, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSyncer(exporter),
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
			)
			jTracer := jtracer.JTracer{OTEL: tracerProvider}
			defer tracerProvider.Shutdown(context.Background())

			ts := initializeTestServer(HandlerOptions.Tracer(&jTracer))
			defer ts.server.Close()

			ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), model.NewTraceID(0, 0x123456abc)).
				Return(makeMockTrace(t), nil).Once()

			var response structuredResponse
			err := getJSON(ts.server.URL+`/api/traces/123456aBC`+testCase.suffix, &response) // trace ID in mixed lower/upper case
			assert.NoError(t, err)
			assert.Len(t, response.Errors, 0)

			assert.Len(t, exporter.GetSpans(), 1, "HTTP request was traced and span reported")
			assert.Equal(t, "/api/traces/{traceID}", exporter.GetSpans()[0].Name)

			traces := extractTraces(t, &response)
			assert.Len(t, traces[0].Spans, 2)
			assert.Len(t, traces[0].Spans[1].References, testCase.numSpanRefs)
		})
	}
}

func TestGetTraceDBFailure(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	assert.Error(t, err)
}

func TestGetTraceNotFound(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	assert.EqualError(t, err, parsedError(404, "trace not found"))
}

func TestGetTraceAdjustmentFailure(t *testing.T) {
	ts := initializeTestServerWithHandler(
		querysvc.QueryServiceOptions{
			Adjuster: adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
				return trace, errAdjustment
			}),
		},
	)
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.EqualValues(t, errAdjustment.Error(), response.Errors[0].Msg)
}

func TestGetTraceBadTraceID(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/chumbawumba`, &response)
	assert.Error(t, err)
}

func TestSearchSuccess(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTrace}, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
}

func TestSearchByTraceIDSuccess(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
	assert.Len(t, response.Data, 2)
}

func TestSearchByTraceIDSuccessWithArchive(t *testing.T) {
	archiveReadMock := &spanstoremocks.Reader{}
	ts := initializeTestServerWithOptions(&tenancy.Manager{}, querysvc.QueryServiceOptions{
		ArchiveSpanReader: archiveReadMock,
	})
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Twice()
	archiveReadMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
	assert.Len(t, response.Data, 2)
}

func TestSearchByTraceIDNotFound(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.Equal(t, structuredError{Msg: "trace not found", TraceID: ui.TraceID("0000000000000001")}, response.Errors[0])
}

func TestSearchByTraceIDFailure(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	whatsamattayou := "https://youtu.be/WrKFOCg13QQ"
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, fmt.Errorf(whatsamattayou)).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1`, &response)
	assert.EqualError(t, err, parsedError(500, whatsamattayou))
}

func TestSearchModelConversionFailure(t *testing.T) {
	ts := initializeTestServerWithOptions(
		&tenancy.Manager{},
		querysvc.QueryServiceOptions{
			Adjuster: adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
				return trace, errAdjustment
			}),
		},
	)
	defer ts.server.Close()
	ts.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTrace}, nil).Once()
	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.EqualValues(t, errAdjustment.Error(), response.Errors[0].Msg)
}

func TestSearchDBFailure(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return(nil, fmt.Errorf("whatsamattayou")).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.EqualError(t, err, parsedError(500, "whatsamattayou"))
}

func TestSearchFailures(t *testing.T) {
	tests := []struct {
		urlStr string
		errMsg string
	}{
		{
			`/api/traces?start=0&end=0&operation=operation&limit=200&minDuration=20ms`,
			parsedError(400, "parameter 'service' is required"),
		},
		{
			`/api/traces?service=service&start=0&end=0&operation=operation&maxDuration=10ms&limit=200&minDuration=20ms`,
			parsedError(400, "'maxDuration' should be greater than 'minDuration'"),
		},
	}
	for _, test := range tests {
		testIndividualSearchFailures(t, test.urlStr, test.errMsg)
	}
}

func testIndividualSearchFailures(t *testing.T, urlStr, errMsg string) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("Query", mock.AnythingOfType("spanstore.TraceQueryParameters")).
		Return([]*model.Trace{}, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+urlStr, &response)
	assert.EqualError(t, err, errMsg)
}

func TestGetServicesSuccess(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	expectedServices := []string{"trifle", "bling"}
	ts.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services", &response)
	assert.NoError(t, err)
	actualServices := make([]string, len(expectedServices))
	for i, s := range response.Data.([]interface{}) {
		actualServices[i] = s.(string)
	}
	assert.Equal(t, expectedServices, actualServices)
}

func TestGetServicesStorageFailure(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services", &response)
	assert.Error(t, err)
}

func TestGetOperationsSuccess(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	expectedOperations := []spanstore.Operation{{Name: ""}, {Name: "get", SpanKind: "server"}}
	ts.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		spanstore.OperationQueryParameters{
			ServiceName: "abc/trifle",
			SpanKind:    "server",
		},
	).Return(expectedOperations, nil).Once()

	var response struct {
		Operations []ui.Operation    `json:"data"`
		Total      int               `json:"total"`
		Limit      int               `json:"limit"`
		Offset     int               `json:"offset"`
		Errors     []structuredError `json:"errors"`
	}

	err := getJSON(ts.server.URL+"/api/operations?service=abc%2Ftrifle&spanKind=server", &response)
	assert.NoError(t, err)
	assert.Equal(t, len(expectedOperations), len(response.Operations))
	for i, op := range response.Operations {
		assert.Equal(t, expectedOperations[i].Name, op.Name)
		assert.Equal(t, expectedOperations[i].SpanKind, op.SpanKind)
	}
}

func TestGetOperationsNoServiceName(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/operations", &response)
	assert.Error(t, err)
}

func TestGetOperationsStorageFailure(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/operations?service=trifle", &response)
	assert.Error(t, err)
}

func TestGetOperationsLegacySuccess(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	expectedOperationNames := []string{"", "get"}
	expectedOperations := []spanstore.Operation{
		{Name: ""},
		{Name: "get", SpanKind: "server"},
		{Name: "get", SpanKind: "client"},
	}

	ts.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(expectedOperations, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services/abc%2Ftrifle/operations", &response)

	assert.NoError(t, err)
	assert.ElementsMatch(t, expectedOperationNames, response.Data.([]interface{}))
}

func TestGetOperationsLegacyStorageFailure(t *testing.T) {
	ts := initializeTestServer()
	defer ts.server.Close()
	ts.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(nil, errStorage).Once()
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services/trifle/operations", &response)
	assert.Error(t, err)
}

func TestGetMetricsSuccess(t *testing.T) {
	mr := &metricsmocks.Reader{}
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.MetricsQueryService(mr),
	}
	ts := initializeTestServer(apiHandlerOptions...)
	defer ts.server.Close()
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
			mockedQueryMethodParamType: "*metricsstore.LatenciesQueryParameters",
		},
		{
			name:                       "call rates",
			urlPath:                    "/api/metrics/calls?service=emailservice",
			mockedQueryMethod:          "GetCallRates",
			mockedQueryMethodParamType: "*metricsstore.CallRateQueryParameters",
		},
		{
			name:                       "error rates",
			urlPath:                    "/api/metrics/errors?service=emailservice",
			mockedQueryMethod:          "GetErrorRates",
			mockedQueryMethodParamType: "*metricsstore.ErrorRateQueryParameters",
		},
		{
			name:                       "error rates with pretty print",
			urlPath:                    "/api/metrics/errors?service=emailservice&prettyPrint=true",
			mockedQueryMethod:          "GetErrorRates",
			mockedQueryMethodParamType: "*metricsstore.ErrorRateQueryParameters",
		},
		{
			name:                       "error rates with spanKinds",
			urlPath:                    "/api/metrics/errors?service=emailservice&spanKind=client",
			mockedQueryMethod:          "GetErrorRates",
			mockedQueryMethodParamType: "*metricsstore.ErrorRateQueryParameters",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare
			mr.On(
				tc.mockedQueryMethod,
				mock.AnythingOfType("*context.valueCtx"),
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

func TestMetricsReaderError(t *testing.T) {
	metricsReader := &metricsmocks.Reader{}
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.MetricsQueryService(metricsReader),
	}
	ts := initializeTestServer(apiHandlerOptions...)
	defer ts.server.Close()

	for _, tc := range []struct {
		name                       string
		urlPath                    string
		mockedQueryMethod          string
		mockedQueryMethodParamType string
		mockedResponse             interface{}
		wantErrorMessage           string
	}{
		{
			urlPath:                    "/api/metrics/calls?service=emailservice",
			mockedQueryMethod:          "GetCallRates",
			mockedQueryMethodParamType: "*metricsstore.CallRateQueryParameters",
			mockedResponse:             nil,
			wantErrorMessage:           "error fetching call rates",
		},
		{
			urlPath:                    "/api/metrics/minstep",
			mockedQueryMethod:          "GetMinStepDuration",
			mockedQueryMethodParamType: "*metricsstore.MinStepDurationQueryParameters",
			mockedResponse:             time.Duration(0),
			wantErrorMessage:           "error fetching min step duration",
		},
	} {
		t.Run(tc.wantErrorMessage, func(t *testing.T) {
			// Prepare
			metricsReader.On(
				tc.mockedQueryMethod,
				mock.AnythingOfType("*context.valueCtx"),
				mock.AnythingOfType(tc.mockedQueryMethodParamType),
			).Return(tc.mockedResponse, fmt.Errorf(tc.wantErrorMessage)).Once()

			// Test
			var response metrics.MetricFamily
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrorMessage)
		})
	}
}

func TestMetricsQueryDisabled(t *testing.T) {
	disabledReader, err := disabled.NewMetricsReader()
	require.NoError(t, err)

	apiHandlerOptions := []HandlerOption{
		HandlerOptions.MetricsQueryService(disabledReader),
	}
	ts := initializeTestServer(apiHandlerOptions...)
	defer ts.server.Close()

	for _, tc := range []struct {
		name             string
		urlPath          string
		wantErrorMessage string
	}{
		{
			name:             "metrics query disabled error returned when fetching latency metrics",
			urlPath:          "/api/metrics/latencies?service=emailservice&quantile=0.95",
			wantErrorMessage: "metrics querying is currently disabled",
		},
		{
			name:             "metrics query disabled error returned when fetching min step duration",
			urlPath:          "/api/metrics/minstep",
			wantErrorMessage: "metrics querying is currently disabled",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Test
			var response interface{}
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrorMessage)
		})
	}
}

func TestGetMinStep(t *testing.T) {
	metricsReader := &metricsmocks.Reader{}
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.MetricsQueryService(metricsReader),
	}
	ts := initializeTestServer(apiHandlerOptions...)
	defer ts.server.Close()
	// Prepare
	metricsReader.On(
		"GetMinStepDuration",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*metricsstore.MinStepDurationQueryParameters"),
	).Return(5*time.Millisecond, nil).Once()

	// Test
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/metrics/minstep", &response)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, float64(5), response.Data)
}

// getJSON fetches a JSON document from a server via HTTP GET
func getJSON(url string, out interface{}) error {
	return getJSONCustomHeaders(url, make(map[string]string), out)
}

func getJSONCustomHeaders(url string, additionalHeaders map[string]string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return execJSON(req, additionalHeaders, out)
}

// postJSON submits a JSON document to a server via HTTP POST and parses response as JSON.
func postJSON(url string, req interface{}, out interface{}) error {
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

// execJSON executes an http request against a server and parses response as JSON
func execJSON(req *http.Request, additionalHeaders map[string]string, out interface{}) error {
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
		return fmt.Errorf("%d error from server: %s", resp.StatusCode, body)
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

func TestSearchTenancyHTTP(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	ts := initializeTestServerWithOptions(
		tenancy.NewManager(&tenancyOptions),
		querysvc.QueryServiceOptions{})
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	require.Error(t, err)
	assert.Equal(t, "401 error from server: missing tenant header", err.Error())
	assert.Len(t, response.Errors, 0)
	assert.Nil(t, response.Data)

	err = getJSONCustomHeaders(
		ts.server.URL+`/api/traces?traceID=1&traceID=2`,
		map[string]string{"x-tenant": "acme"},
		&response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
	assert.Len(t, response.Data, 2)
}

func TestSearchTenancyRejectionHTTP(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	ts := initializeTestServerWithOptions(
		tenancy.NewManager(&tenancyOptions),
		querysvc.QueryServiceOptions{})
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Twice()

	req, err := http.NewRequest(http.MethodGet, ts.server.URL+`/api/traces?traceID=1&traceID=2`, nil)
	assert.NoError(t, err)
	req.Header.Add("Accept", "application/json")
	// We don't set tenant header
	resp, err := httpClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	tm := tenancy.NewManager(&tenancyOptions)
	req.Header.Set(tm.Header, "acme")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// Skip unmarshal of response; it is enough that it succeeded
}

func TestSearchTenancyFlowTenantHTTP(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	ts := initializeTestServerWithOptions(
		tenancy.NewManager(&tenancyOptions),
		querysvc.QueryServiceOptions{})
	defer ts.server.Close()
	ts.spanReader.On("GetTrace", mock.MatchedBy(func(v interface{}) bool {
		ctx, ok := v.(context.Context)
		if !ok || tenancy.GetTenant(ctx) != "acme" {
			return false
		}
		return true
	}), mock.AnythingOfType("model.TraceID")).Return(mockTrace, nil).Twice()
	ts.spanReader.On("GetTrace", mock.MatchedBy(func(v interface{}) bool {
		ctx, ok := v.(context.Context)
		if !ok || tenancy.GetTenant(ctx) != "megacorp" {
			return false
		}
		return true
	}), mock.AnythingOfType("model.TraceID")).Return(nil, errStorage).Once()

	var responseAcme structuredResponse
	err := getJSONCustomHeaders(
		ts.server.URL+`/api/traces?traceID=1&traceID=2`,
		map[string]string{"x-tenant": "acme"},
		&responseAcme)
	assert.NoError(t, err)
	assert.Len(t, responseAcme.Errors, 0)
	assert.Len(t, responseAcme.Data, 2)

	var responseMegacorp structuredResponse
	err = getJSONCustomHeaders(
		ts.server.URL+`/api/traces?traceID=1&traceID=2`,
		map[string]string{"x-tenant": "megacorp"},
		&responseMegacorp)
	assert.Contains(t, err.Error(), "storage error")
	assert.Len(t, responseMegacorp.Errors, 0)
	assert.Nil(t, responseMegacorp.Data)
}
