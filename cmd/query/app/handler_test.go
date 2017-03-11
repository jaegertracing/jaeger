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
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/uber-go/zap"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/model/adjuster"
	ui "github.com/uber/jaeger/model/json"
	depsmocks "github.com/uber/jaeger/storage/dependencystore/mocks"
	"github.com/uber/jaeger/storage/spanstore"
	spanstoremocks "github.com/uber/jaeger/storage/spanstore/mocks"
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
				SpanID:  model.SpanID(1),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.SpanID(2),
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

func initializeTestServerWitHandler() (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader, *APIHandler) {
	return initializeTestServerWithOptions(
		HandlerOptions.Logger(zap.New(zap.NullEncoder())),
		HandlerOptions.Prefix(defaultHTTPPrefix),
		HandlerOptions.QueryLookbackDuration(defaultTraceQueryLookbackDuration),
	)
}

func initializeTestServerWithOptions(options ...HandlerOption) (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader, *APIHandler) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	r := mux.NewRouter()
	handler := NewAPIHandler(readStorage, dependencyStorage, options...)
	handler.RegisterRoutes(r)
	return httptest.NewServer(r), readStorage, dependencyStorage, handler
}

func initializeTestServer() (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader) {
	https, sr, dr, _ := initializeTestServerWitHandler()
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
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "trace not found"))
}

func TestGetTraceAdjustmentFailure(t *testing.T) {
	server, readMock, _, handler := initializeTestServerWitHandler()
	handler.adjuster = adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
		return trace, errAdjustment
	})
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

func TestSearchModelConversionFailure(t *testing.T) {
	server, readMock, _, _ := initializeTestServerWithOptions(
		HandlerOptions.Adjusters([]adjuster.Adjuster{
			adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
				return trace, errAdjustment
			})}),
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
	assert.EqualError(
		t, err,
		`500 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":500,"msg":"whatsamattayou"}]}`+"\n")
}

func TestSearchFailures(t *testing.T) {
	tests := []struct {
		urlStr string
		errMsg string
	}{
		{
			`/api/traces?start=0&end=0&operation=operation&limit=200&minDuration=20ms`,
			`400 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":400,"msg":"Parameter 'service' is required"}]}`,
		},
		{
			`/api/traces?service=service&start=0&end=0&operation=operation&maxDuration=10ms&limit=200&minDuration=20ms`,
			`400 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":400,"msg":"'maxDuration' should be greater than 'minDuration'"}]}`,
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
	assert.EqualError(t, err, errMsg+"\n")
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
	mock.On("GetOperations", "trifle").Return(expectedOperations, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/operations?service=trifle", &response)
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
	mock.On("GetOperations", "trifle").Return(expectedOperations, nil).Once()

	var response structuredResponse
	err := getJSON(server.URL+"/api/services/trifle/operations", &response)
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
