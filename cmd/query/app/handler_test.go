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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
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
)

func initializeTestServerWitHandler() (*httptest.Server, *spanstoremocks.Reader, *depsmocks.Reader, *APIHandler) {
	return initializeTestServerWithOptions(
		HandlerOptions.Logger(zap.New(zap.NullEncoder())),
		HandlerOptions.Prefix(DefaultHTTPPrefix),
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

func TestGetTraceSuccess(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).Return(&model.Trace{
		Spans:    []*model.Span{},
		Warnings: []string{},
	}, nil).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/123456`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
}

func TestGetTraceDBFailure(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces/123456`, &response)
	assert.Error(t, err)
}

func TestGetTraceAdjustmentFailure(t *testing.T) {
	server, readMock, _, handler := initializeTestServerWitHandler()
	handler.adjuster = adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
		return trace, errAdjustment
	})
	defer server.Close()
	readMock.On("GetTrace", mock.AnythingOfType("model.TraceID")).Return(&model.Trace{
		Spans:    []*model.Span{},
		Warnings: []string{},
	}, nil).Times(1)

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
	readMock.On("FindTraces", mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return([]*model.Trace{
		{
			Spans:    []*model.Span{},
			Warnings: []string{},
		},
	}, nil).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 0)
}

func TestSearchModelConversionFailure(t *testing.T) {
	server, readMock, _, _ := initializeTestServerWithOptions(HandlerOptions.Adjusters([]adjuster.Adjuster{
		adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
			return trace, errAdjustment
		})}),
	)
	defer server.Close()
	readMock.On("FindTraces", mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return([]*model.Trace{
		{
			Spans:    []*model.Span{},
			Warnings: []string{},
		},
	}, nil).Times(1)
	var response structuredResponse
	err := getJSON(server.URL+`/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms`, &response)
	assert.NoError(t, err)
	assert.Len(t, response.Errors, 1)
	assert.EqualValues(t, errAdjustment.Error(), response.Errors[0].Msg)
}

func TestSearchDBFailure(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	readMock.On("FindTraces", mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return(nil, fmt.Errorf("whatsamattayou")).Times(1)

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
	readMock.On("Query", mock.AnythingOfType("spanstore.TraceQueryParameters")).Return([]*model.Trace{}, nil).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+urlStr, &response)
	assert.EqualError(t, err, errMsg+"\n")
}

func TestGetServicesSuccess(t *testing.T) {
	server, readMock, _ := initializeTestServer()
	defer server.Close()
	expectedServices := []string{"trifle", "bling"}
	readMock.On("GetServices").Return(expectedServices, nil).Times(1)

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
	mock.On("GetServices").Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/services", &response)
	assert.Error(t, err)
}

func TestGetOperationsSuccess(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	expectedOperations := []string{"", "get"}
	mock.On("GetOperations", "trifle").Return(expectedOperations, nil).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/services/operations?service=trifle", &response)
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
	err := getJSON(server.URL+"/api/services/operations", &response)
	assert.Error(t, err)
}

func TestGetOperationsStorageFailure(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	mock.On("GetOperations", "trifle").Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/services/operations?service=trifle", &response)
	assert.Error(t, err)
}

func TestGetOperationsLegacySuccess(t *testing.T) {
	server, mock, _ := initializeTestServer()
	defer server.Close()
	expectedOperations := []string{"", "get"}
	mock.On("GetOperations", "trifle").Return(expectedOperations, nil).Times(1)

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
	mock.On("GetOperations", "trifle").Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/services/trifle/operations", &response)
	assert.Error(t, err)
}

func TestDeduplicateDependencies(t *testing.T) {
	handler := &APIHandler{}
	tests := []struct {
		description string
		input       []model.DependencyLink
		expected    []ui.DependencyLink
	}{
		{
			"Single parent and child",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
		},
		{
			"Single parent, multiple children",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
		},
		{
			"multiple parents, single child",
			[]model.DependencyLink{
				{
					Parent:    "Hador",
					Child:     "Glóredhel",
					CallCount: 3,
				},
				{
					Parent:    "Gildis",
					Child:     "Glóredhel",
					CallCount: 9,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Hador",
					Child:     "Glóredhel",
					CallCount: 3,
				},
				{
					Parent:    "Gildis",
					Child:     "Glóredhel",
					CallCount: 9,
				},
			},
		},
		{
			"single parent, multiple children with duplicates",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 473,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
		},
	}

	for _, test := range tests {
		actual := handler.deduplicateDependencies(test.input)
		sort.Sort(DependencyLinks(actual))
		expected := test.expected
		sort.Sort(DependencyLinks(expected))
		assert.Equal(t, expected, actual, test.description)
	}
}

type DependencyLinks []ui.DependencyLink

func (slice DependencyLinks) Len() int {
	return len(slice)
}

func (slice DependencyLinks) Less(i, j int) bool {
	if slice[i].Parent != slice[j].Parent {
		return slice[i].Parent < slice[j].Parent
	}
	if slice[i].Child != slice[j].Child {
		return slice[i].Child < slice[j].Child
	}
	return slice[i].CallCount < slice[j].CallCount
}

func (slice DependencyLinks) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func TestFilterDependencies(t *testing.T) {
	handler := &APIHandler{}
	tests := []struct {
		description  string
		service      string
		dependencies []model.DependencyLink
		expected     []model.DependencyLink
	}{
		{
			"No services filtered for %s",
			"Drogo",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
		},
		{
			"No services filtered for empty string",
			"",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
		},
		{
			"All services filtered away for %s",
			"Dáin I",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]model.DependencyLink(nil),
		},
		{
			"Filter by parent %s",
			"Dáin I",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
		},
		{
			"Filter by child %s",
			"Frór",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
			},
		},
	}

	for _, test := range tests {
		actual := handler.filterDependenciesByService(test.dependencies, test.service)
		assert.Equal(t, test.expected, actual, test.description, test.service)
	}
}

func TestGetDependenciesSuccess(t *testing.T) {
	server, _, mock := initializeTestServer()
	defer server.Close()
	expectedDependencies := []model.DependencyLink{{Parent: "killer", Child: "queen", CallCount: 12}}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	mock.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(expectedDependencies, nil).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=1476374248550&service=queen", &response)
	assert.NotEmpty(t, response.Data)
	data := response.Data.([]interface{})[0]
	actual := data.(map[string]interface{})
	assert.Equal(t, actual["parent"], "killer")
	assert.Equal(t, actual["child"], "queen")
	assert.Equal(t, actual["callCount"], 12.00) //recovered type is float
	assert.NoError(t, err)
}

func TestGetDependenciesCassandraFailure(t *testing.T) {
	server, _, mock := initializeTestServer()
	defer server.Close()
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	mock.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=1476374248550&service=testing", &response)
	assert.Error(t, err)
}

func TestGetDependenciesEndTimeParsingFailure(t *testing.T) {
	server, _, _ := initializeTestServer()
	defer server.Close()

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=shazbot&service=testing", &response)
	assert.Error(t, err)
}

func TestGetDependenciesLookbackParsingFailure(t *testing.T) {
	server, _, _ := initializeTestServer()
	defer server.Close()

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=1476374248550&service=testing&lookback=shazbot", &response)
	assert.Error(t, err)
}

// GetJSON fetches a JSON document from a server using the given client
func getJSON(url string, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
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
