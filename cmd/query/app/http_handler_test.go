// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
	"os"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metricstore/disabled"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	metricsmocks "github.com/jaegertracing/jaeger/storage/metricstore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
	depsmocks "github.com/jaegertracing/jaeger/storage_v2/depstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

type IoReaderMock struct {
	mock.Mock
}

func (m *IoReaderMock) Read(b []byte) (int, error) {
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

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
				HandlerOptions.QueryLookbackDuration(defaultTraceQueryLookbackDuration),
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
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	traceReader := v1adapter.NewTraceReader(readStorage)
	qs := querysvc.NewQueryService(traceReader, dependencyStorage, queryOptions)
	r := NewRouter()
	apiHandler := NewAPIHandler(qs, options...)
	apiHandler.RegisterRoutes(r)
	ts := &testServer{
		server:           httptest.NewServer(tenancy.ExtractTenantHTTPHandler(tenancyMgr, r)),
		spanReader:       readStorage,
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
	spanReader       *spanstoremocks.Reader
	dependencyReader *depsmocks.Reader
	handler          *APIHandler
	server           *httptest.Server
}

func withTestServer(t *testing.T, doTest func(s *testServer), queryOptions querysvc.QueryServiceOptions, options ...HandlerOption) {
	ts := initializeTestServerWithOptions(t, &tenancy.Manager{}, queryOptions, options...)
	doTest(ts)
}

func TestGetTraceSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
}

func extractTraces(t *testing.T, response *structuredResponse) []ui.Trace {
	var traces []ui.Trace
	b, err := json.Marshal(response.Data)
	require.NoError(t, err)
	err = json.Unmarshal(b, &traces)
	require.NoError(t, err)
	return traces
}

// TestGetTraceDedupeSuccess partially verifies that the standard adjusteres
// are defined in correct order.
func TestGetTraceDedupeSuccess(t *testing.T) {
	dupedMockTrace := &model.Trace{
		Spans:    append(mockTrace.Spans, mockTrace.Spans...),
		Warnings: []string{},
	}

	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(dupedMockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
	traces := extractTraces(t, &response)
	assert.Len(t, traces[0].Spans, 2)
	for _, span := range traces[0].Spans {
		assert.Empty(t, span.Warnings, "clock skew adjuster would've complained about dupes")
	}
}

func TestGetTraceWithTimeWindowSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), spanstore.GetTraceParameters{
		TraceID:   mockTraceID,
		StartTime: time.UnixMicro(1),
		EndTime:   time.UnixMicro(2),
	}).Return(mockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/`+mockTraceID.String()+`?start=1&end=2`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
}

func TestLogOnServerError(t *testing.T) {
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	readStorage := &spanstoremocks.Reader{}
	traceReader := v1adapter.NewTraceReader(readStorage)
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

func TestGetTrace(t *testing.T) {
	testCases := []struct {
		suffix      string
		raw         bool
		numSpanRefs int
	}{
		{suffix: "", numSpanRefs: 0},
		{suffix: "?raw=true", raw: true, numSpanRefs: 1}, // bad span reference is not filtered out
		{suffix: "?raw=false", raw: false, numSpanRefs: 0},
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

			ts := initializeTestServer(t, HandlerOptions.Tracer(jTracer.OTEL))

			ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), spanstore.GetTraceParameters{
				TraceID:   model.NewTraceID(0, 0x123456abc),
				RawTraces: testCase.raw,
			}).
				Return(makeMockTrace(t), nil).Once()

			var response structuredResponse
			err := getJSON(ts.server.URL+`/api/traces/123456aBC`+testCase.suffix, &response) // trace ID in mixed lower/upper case
			require.NoError(t, err)
			assert.Empty(t, response.Errors)

			traces := extractTraces(t, &response)
			assert.Len(t, traces[0].Spans, 2)
			assert.Len(t, traces[0].Spans[1].References, testCase.numSpanRefs)
		})
	}
}

func TestGetTraceDBFailure(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	require.Error(t, err)
}

func TestGetTraceNotFound(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/123456`, &response)
	require.EqualError(t, err, parsedError(404, "trace not found"))
}

func TestGetTraceBadTraceID(t *testing.T) {
	ts := initializeTestServer(t)

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces/chumbawumba`, &response)
	require.Error(t, err)
}

func TestGetTraceBadTimeWindow(t *testing.T) {
	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "Bad start time",
			query: "start=a",
		},
		{
			name:  "Bad end time",
			query: "end=b",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := initializeTestServer(t)
			var response structuredResponse
			err := getJSON(ts.server.URL+`/api/traces/123456?`+tc.query, &response)
			require.Error(t, err)
			require.ErrorContains(t, err, "400 error from server")
			require.ErrorContains(t, err, "unable to parse param")
		})
	}
}

func TestSearchSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTrace}, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
}

func TestSearchByTraceIDSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
	assert.Len(t, response.Data, 2)
}

func TestSearchByTraceIDWithTimeWindowSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	expectedQuery1 := spanstore.GetTraceParameters{
		TraceID:   mockTraceID,
		StartTime: time.UnixMicro(1),
		EndTime:   time.UnixMicro(2),
	}
	traceId2 := model.NewTraceID(0, 456789)
	expectedQuery2 := spanstore.GetTraceParameters{
		TraceID:   traceId2,
		StartTime: time.UnixMicro(1),
		EndTime:   time.UnixMicro(2),
	}
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), expectedQuery1).
		Return(mockTrace, nil)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), expectedQuery2).
		Return(mockTrace, nil)

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=`+mockTraceID.String()+`&traceID=`+traceId2.String()+`&start=1&end=2`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
	assert.Len(t, response.Data, 2)
}

func TestSearchTraceBadTimeWindow(t *testing.T) {
	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "Bad start time",
			query: "start=a",
		},
		{
			name:  "Bad end time",
			query: "end=b",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := initializeTestServer(t)
			var response structuredResponse
			err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2&`+tc.query, &response)
			require.Error(t, err)
			require.ErrorContains(t, err, "400 error from server")
			require.ErrorContains(t, err, "unable to parse param")
		})
	}
}

func TestSearchByTraceIDSuccessWithArchive(t *testing.T) {
	archiveReadMock := &spanstoremocks.Reader{}
	ts := initializeTestServerWithOptions(t, &tenancy.Manager{}, querysvc.QueryServiceOptions{
		ArchiveSpanReader: archiveReadMock,
	})
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Twice()
	archiveReadMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
	assert.Len(t, response.Data, 2)
}

func TestSearchByTraceIDSuccessWithArchiveAndTimeWindow(t *testing.T) {
	archiveReadMock := &spanstoremocks.Reader{}
	ts := initializeTestServerWithOptions(t, &tenancy.Manager{}, querysvc.QueryServiceOptions{
		ArchiveSpanReader: archiveReadMock,
	})
	expectedQuery := spanstore.GetTraceParameters{
		TraceID:   mockTraceID,
		StartTime: time.UnixMicro(1),
		EndTime:   time.UnixMicro(2),
	}
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), expectedQuery).
		Return(nil, spanstore.ErrTraceNotFound)
	archiveReadMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), expectedQuery).
		Return(mockTrace, nil)

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=`+mockTraceID.String()+`&start=1&end=2`, &response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
	assert.Len(t, response.Data, 1)
}

func TestSearchByTraceIDNotFound(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1`, &response)
	require.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.Equal(t, structuredError{Msg: "trace not found", TraceID: ui.TraceID("0000000000000001")}, response.Errors[0])
}

func TestSearchByTraceIDFailure(t *testing.T) {
	ts := initializeTestServer(t)
	whatsamattayou := "whatsamattayou"
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, errors.New(whatsamattayou)).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1`, &response)
	require.EqualError(t, err, parsedError(500, whatsamattayou))
}

func TestSearchDBFailure(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return(nil, errors.New("whatsamattayou")).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	require.EqualError(t, err, parsedError(500, "whatsamattayou"))
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
	ts := initializeTestServer(t)
	ts.spanReader.On("Query", mock.AnythingOfType("spanstore.TraceQueryParameters")).
		Return([]*model.Trace{}, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+urlStr, &response)
	require.EqualError(t, err, errMsg)
}

func TestGetServicesSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	expectedServices := []string{"trifle", "bling"}
	ts.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services", &response)
	require.NoError(t, err)
	actualServices := make([]string, len(expectedServices))
	for i, s := range response.Data.([]any) {
		actualServices[i] = s.(string)
	}
	assert.Equal(t, expectedServices, actualServices)
}

func TestGetServicesStorageFailure(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services", &response)
	require.Error(t, err)
}

func TestGetOperationsSuccess(t *testing.T) {
	ts := initializeTestServer(t)
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
	require.NoError(t, err)
	assert.Equal(t, len(expectedOperations), len(response.Operations))
	for i, op := range response.Operations {
		assert.Equal(t, expectedOperations[i].Name, op.Name)
		assert.Equal(t, expectedOperations[i].SpanKind, op.SpanKind)
	}
}

func TestGetOperationsNoServiceName(t *testing.T) {
	ts := initializeTestServer(t)
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/operations", &response)
	require.Error(t, err)
}

func TestGetOperationsStorageFailure(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/operations?service=trifle", &response)
	require.Error(t, err)
}

func TestGetOperationsLegacySuccess(t *testing.T) {
	ts := initializeTestServer(t)
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

	require.NoError(t, err)
	assert.ElementsMatch(t, expectedOperationNames, response.Data.([]any))
}

func TestGetOperationsLegacyStorageFailure(t *testing.T) {
	ts := initializeTestServer(t)
	ts.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(nil, errStorage).Once()
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/services/trifle/operations", &response)
	require.Error(t, err)
}

func TestTransformOTLPSuccess(t *testing.T) {
	reformat := func(in []byte) []byte {
		obj := new(any)
		require.NoError(t, json.Unmarshal(in, obj))
		// format json similar to `jq .`
		out, err := json.MarshalIndent(obj, "", "  ")
		require.NoError(t, err)
		return out
	}
	withTestServer(t, func(ts *testServer) {
		inFile, err := os.Open("./fixture/otlp2jaeger-in.json")
		require.NoError(t, err)

		resp, err := ts.server.Client().Post(ts.server.URL+"/api/transform", "application/json", inFile)
		require.NoError(t, err)

		responseBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		responseBytes = reformat(responseBytes)

		expectedBytes, err := os.ReadFile("./fixture/otlp2jaeger-out.json")
		require.NoError(t, err)
		expectedBytes = reformat(expectedBytes)

		assert.Equal(t, string(expectedBytes), string(responseBytes))
	}, querysvc.QueryServiceOptions{})
}

func TestTransformOTLPReadError(t *testing.T) {
	withTestServer(t, func(ts *testServer) {
		bytesReader := &IoReaderMock{}
		bytesReader.On("Read", mock.AnythingOfType("[]uint8")).Return(0, errors.New("Mocked error"))
		_, err := ts.server.Client().Post(ts.server.URL+"/api/transform", "application/json", bytesReader)
		require.Error(t, err)
	}, querysvc.QueryServiceOptions{})
}

func TestTransformOTLPBadPayload(t *testing.T) {
	withTestServer(t, func(ts *testServer) {
		response := new(any)
		request := "Bad Payload"
		err := postJSON(ts.server.URL+"/api/transform", request, response)
		require.ErrorContains(t, err, "cannot unmarshal OTLP")
	}, querysvc.QueryServiceOptions{})
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
		{
			urlPath:                    "/api/metrics/minstep",
			mockedQueryMethod:          "GetMinStepDuration",
			mockedQueryMethodParamType: "*metricstore.MinStepDurationQueryParameters",
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
			var response any
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			assert.ErrorContains(t, err, tc.wantErrorMessage)
		})
	}
}

func TestGetMinStep(t *testing.T) {
	metricsReader := &metricsmocks.Reader{}
	ts := initializeTestServer(t, HandlerOptions.MetricsQueryService(metricsReader))
	defer ts.server.Close()
	// Prepare
	metricsReader.On(
		"GetMinStepDuration",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*metricstore.MinStepDurationQueryParameters"),
	).Return(5*time.Millisecond, nil).Once()

	// Test
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/metrics/minstep", &response)

	// Verify
	require.NoError(t, err)
	assert.InDelta(t, float64(5), response.Data, 0.01)
}

// getJSON fetches a JSON document from a server via HTTP GET
func getJSON(url string, out any) error {
	return getJSONCustomHeaders(url, make(map[string]string), out)
}

func getJSONCustomHeaders(url string, additionalHeaders map[string]string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
	ts := initializeTestServerWithOptions(t,
		tenancy.NewManager(&tenancyOptions),
		querysvc.QueryServiceOptions{})
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(ts.server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	require.Error(t, err)
	assert.Equal(t, "401 error from server: missing tenant header", err.Error())
	assert.Empty(t, response.Errors)
	assert.Nil(t, response.Data)

	err = getJSONCustomHeaders(
		ts.server.URL+`/api/traces?traceID=1&traceID=2`,
		map[string]string{"x-tenant": "acme"},
		&response)
	require.NoError(t, err)
	assert.Empty(t, response.Errors)
	assert.Len(t, response.Data, 2)
}

func TestSearchTenancyRejectionHTTP(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	ts := initializeTestServerWithOptions(t, tenancy.NewManager(&tenancyOptions), querysvc.QueryServiceOptions{})
	ts.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Twice()

	req, err := http.NewRequest(http.MethodGet, ts.server.URL+`/api/traces?traceID=1&traceID=2`, nil)
	require.NoError(t, err)
	req.Header.Add("Accept", "application/json")
	// We don't set tenant header
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	tm := tenancy.NewManager(&tenancyOptions)
	req.Header.Set(tm.Header, "acme")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// Skip unmarshal of response; it is enough that it succeeded
}

func TestSearchTenancyFlowTenantHTTP(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	ts := initializeTestServerWithOptions(t, tenancy.NewManager(&tenancyOptions), querysvc.QueryServiceOptions{})
	ts.spanReader.On("GetTrace", mock.MatchedBy(func(v any) bool {
		ctx, ok := v.(context.Context)
		if !ok || tenancy.GetTenant(ctx) != "acme" {
			return false
		}
		return true
	}), mock.AnythingOfType("spanstore.GetTraceParameters")).Return(mockTrace, nil).Twice()
	ts.spanReader.On("GetTrace", mock.MatchedBy(func(v any) bool {
		ctx, ok := v.(context.Context)
		if !ok || tenancy.GetTenant(ctx) != "megacorp" {
			return false
		}
		return true
	}), mock.AnythingOfType("spanstore.GetTraceParameters")).Return(nil, errStorage).Once()

	var responseAcme structuredResponse
	err := getJSONCustomHeaders(
		ts.server.URL+`/api/traces?traceID=1&traceID=2`,
		map[string]string{"x-tenant": "acme"},
		&responseAcme)
	require.NoError(t, err)
	assert.Empty(t, responseAcme.Errors)
	assert.Len(t, responseAcme.Data, 2)

	var responseMegacorp structuredResponse
	err = getJSONCustomHeaders(
		ts.server.URL+`/api/traces?traceID=1&traceID=2`,
		map[string]string{"x-tenant": "megacorp"},
		&responseMegacorp)
	require.ErrorContains(t, err, "storage error")
	assert.Empty(t, responseMegacorp.Errors)
	assert.Nil(t, responseMegacorp.Data)
}
