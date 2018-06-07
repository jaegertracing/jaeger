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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	jaeger "github.com/uber/jaeger-client-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	ui "github.com/jaegertracing/jaeger/model/json"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	errStorageMsg = "Storage error"
	errStorage    = errors.New(errStorageMsg)
	errAdjustment = errors.New("Adjustment error")

	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}

	mockTraceID = model.TraceID{Low: 123456}
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

func initializeTestServerWithHandler(options ...HandlerOption) (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader, *APIHandler) {
	return initializeTestServerWithOptions(
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

func initializeTestServerWithOptions(options ...HandlerOption) (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader, *APIHandler) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	r := NewRouter()
	handler := NewAPIHandler(readStorage, dependencyStorage, options...)
	handler.RegisterRoutes(r)
	return httptest.NewServer(r), readStorage, dependencyStorage, handler
}

func initializeTestServer(options ...HandlerOption) (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader) {
	https, sr, dr, _ := initializeTestServerWithHandler(options...)
	return https, sr, dr
}

type testServer struct {
	spanReader       *spanstoremocks.Reader
	dependencyReader *depsmocks.Reader
	handler          *APIHandler
	server           *httptest.Server
}

func withTestServer(t *testing.T, doTest func(s *testServer), options ...HandlerOption) {
	server, spanReader, depReader, handler := initializeTestServerWithOptions(options...)
	s := &testServer{
		spanReader:       spanReader,
		dependencyReader: depReader,
		handler:          handler,
		server:           server,
	}
	defer server.Close()
	doTest(s)
}

func TestGetTraceSuccess(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/123456`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
}

func TestPrettyPrint(t *testing.T) {
	data := struct{ Data string }{Data: "Bender"}

	testCases := []struct {
		param  string
		output string
	}{
		{output: `{"Data":"Bender"}`},
		{param: "?prettyPrint=false", output: `{"Data":"Bender"}`},
		{param: "?prettyPrint=x", output: "{\n    \"Data\": \"Bender\"\n}"},
	}

	get := func(url string) string {
		res, err := http.Get(url)
		require.NoError(t, err)
		body, err := ioutil.ReadAll(res.Body)
		require.NoError(t, err)
		return string(body)
	}

	for _, testCase := range testCases {
		t.Run(testCase.param, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				new(APIHandler).writeJSON(w, r, &data)
			}))
			defer server.Close()

			out := get(server.URL + testCase.param)
			assert.Equal(t, testCase.output, out)
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
		bytes, err := json.Marshal(mockTrace)
		require.NoError(t, err)
		var trace *model.Trace
		require.NoError(t, json.Unmarshal(bytes, &trace))
		trace.Spans[1].References = []model.SpanRef{
			{TraceID: model.TraceID{High: 0, Low: 0}},
		}
		return trace
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
			reporter := jaeger.NewInMemoryReporter()
			jaegerTracer, jaegerCloser := jaeger.NewTracer("test", jaeger.NewConstSampler(true), reporter)
			defer jaegerCloser.Close()

			server, readMock, _ := initializeTestServer(HandlerOptions.Tracer(jaegerTracer))
			defer server.Close()

			readMock.On("GetTrace", model.TraceID{Low: 0x123456abc}).
				Return(makeMockTrace(t), nil).Once()

			var response structuredResponse
			err := getJSON(server.URL+`/api/traces/123456aBC`+testCase.suffix, &response) // trace ID in mixed lower/upper case
			assert.NoError(t, err)
			assert.Len(t, response.Errors, 0)

			assert.Len(t, reporter.GetSpans(), 1, "HTTP request was traced and span reported")
			assert.Equal(t, "/api/traces/{traceID}", reporter.GetSpans()[0].(*jaeger.Span).OperationName())

			traces := extractTraces(t, &response)
			assert.Len(t, traces[0].Spans, 2)
			assert.Len(t, traces[0].Spans[1].References, testCase.numSpanRefs)
		})
	}
}

func TestGetTraceDBFailure(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/123456`, &response)
	assert.Error(t, err)
}

func TestGetTraceNotFound(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/123456`, &response)
	assert.EqualError(t, err, parsedError(404, "trace not found"))
}

func TestGetTraceAdjustmentFailure(t *testing.T) {
	server, readMock, _, _ := initializeTestServerWithHandler(
		HandlerOptions.Adjusters(
			adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
				return trace, errAdjustment
			}),
		),
	)
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/123456`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.EqualValues(t, errAdjustment.Error(), response.Errors[0].Msg)
}

func TestGetTraceBadTraceID(t *testing.T) {
	server, _, _ := initializeTestServer()
	defer server.Close()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/chumbawumba`, &response)
	assert.Error(t, err)
}

func TestSearchSuccess(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("FindTraces", mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTrace}, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
}

func TestSearchByTraceIDSuccess(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Twice()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?traceID=1&traceID=2`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
	assert.Len(t, response.Data, 2)
}

func TestSearchByTraceIDNotFound(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?traceID=1`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.Equal(t, structuredError{Msg: "trace not found", TraceID: ui.TraceID("1")}, response.Errors[0])
}

func TestSearchByTraceIDFailure(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	whatsamattayou := "https://youtu.be/WrKFOCg13QQ"
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(nil, fmt.Errorf(whatsamattayou)).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?traceID=1`, &response)
	assert.EqualError(t, err, parsedError(500, whatsamattayou))
}

func TestSearchModelConversionFailure(t *testing.T) {
	server, readMock, _, _ := initializeTestServerWithOptions(
		HandlerOptions.Adjusters(
			adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
				return trace, errAdjustment
			}),
		),
	)
	defer server.Close()
	readMock.On("FindTraces", mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTrace}, nil).Once()
	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.EqualValues(t, errAdjustment.Error(), response.Errors[0].Msg)
}

func TestSearchDBFailure(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("FindTraces", mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return(nil, fmt.Errorf("whatsamattayou")).Once()

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.EqualError(t, err, parsedError(500, "whatsamattayou"))
}

func TestSearchFailures(t *testing.T) {
	tests := []struct {
		urlStr string
		errMsg string
	}{
		{
			`/api/traces?start=0&end=0&operation=operation&limit=200&minDuration=20ms`,
			parsedError(400, "Parameter 'service' is required"),
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
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("Query", mock.AnythingOfType("spanstore.TraceQueryParameters")).
		Return([]*model.Trace{}, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+urlStr, &response)
	assert.EqualError(t, err, errMsg)
}

func TestGetServicesSuccess(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	expectedServices := []string{"trifle", "bling"}
	readMock.On("GetServices").Return(expectedServices, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/services", &response)
	assert.NoError(t, err)
	actualServices := make([]string, len(expectedServices))
	for i, s := range response.Data.([]interface{}) {
		actualServices[i] = s.(string)
	}
	assert.Equal(t, expectedServices, actualServices)
}

func TestGetServicesStorageFailure(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	mock.On("GetServices").Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/services", &response)
	assert.Error(t, err)
}

func TestGetOperationsSuccess(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	expectedOperations := []string{"", "get"}
	mock.On("GetOperations", "abc/trifle").Return(expectedOperations, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/operations?service=abc%2Ftrifle", &response)
	assert.NoError(t, err)
	actualOperations := make([]string, len(expectedOperations))
	for i, s := range response.Data.([]interface{}) {
		actualOperations[i] = s.(string)
	}
	assert.Equal(t, expectedOperations, actualOperations)
}

func TestGetOperationsNoServiceName(t *testing.T) {
	server, _, _ := initializeTestServer()
	defer server.Close()

	var response structuredResponse
	err := getJSON(server.URL+"/api/operations", &response)
	assert.Error(t, err)
}

func TestGetOperationsStorageFailure(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	mock.On("GetOperations", "trifle").Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/operations?service=trifle", &response)
	assert.Error(t, err)
}

func TestGetOperationsLegacySuccess(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	expectedOperations := []string{"", "get"}
	mock.On("GetOperations", "abc/trifle").Return(expectedOperations, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/services/abc%2Ftrifle/operations", &response)
	assert.NoError(t, err)
	actualOperations := make([]string, len(expectedOperations))
	for i, s := range response.Data.([]interface{}) {
		actualOperations[i] = s.(string)
	}
	assert.Equal(t, expectedOperations, actualOperations)
}

func TestGetOperationsLegacyStorageFailure(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	mock.On("GetOperations", "trifle").Return(nil, errStorage).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/services/trifle/operations", &response)
	assert.Error(t, err)
}

// getJSON fetches a JSON document from a server via HTTP GET
func getJSON(url string, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	return execJSON(req, out)
}

// postJSON submits a JSON document to a server via HTTP POST and parses response as JSON.
func postJSON(url string, req interface{}, out interface{}) error {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(req); err != nil {
		return err
	}
	r, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return err
	}
	return execJSON(r, out)
}

// execJSON executes an http request against a server and parses response as JSON
func execJSON(req *http.Request, out interface{}) error {
	req.Header.Add("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode > 399 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("%d error from server: %s", resp.StatusCode, body)
	}

	if out == nil {
		io.Copy(ioutil.Discard, resp.Body)
		return nil
	}

	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(out)
}

// Generates a JSON response that the server should produce given a certain error code and error.
func parsedError(code int, err string) string {
	return fmt.Sprintf(`%d error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":%d,"msg":"%s"}]}`+"\n", code, code, err)
}
