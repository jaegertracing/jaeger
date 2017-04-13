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

package services

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-client-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/crossdock/services/mocks"
	ui "github.com/uber/jaeger/model/json"
)

func TestCreateTraceRequest(t *testing.T) {
	handler := NewTraceHandler(nil, nil, zap.NewNop())
	req := handler.createTraceRequest(jaeger.SamplerTypeConst, "op", 23)
	assert.Equal(t, "op", req.Operation)
	assert.Equal(t, 23, req.Count)
	assert.Equal(t, jaeger.SamplerTypeConst, req.Type)
	assert.Len(t, req.Tags, 1)
}

func TestExpectedTagsExist(t *testing.T) {
	actual := map[string]string{"key": "value"}
	assert.True(t, expectedTagsExist(actual, actual))
	assert.False(t, expectedTagsExist(map[string]string{"key": "value1"}, actual))
	assert.False(t, expectedTagsExist(map[string]string{"key1": "value1"}, actual))
}

func TestConvertTagsIntoMap(t *testing.T) {
	tags := []ui.KeyValue{{Key: "key", Type: ui.StringType, Value: "value"}}

	actual := convertTagsIntoMap(tags)
	assert.Equal(t, map[string]string{"key": "value"}, actual)

	tags = []ui.KeyValue{{Key: "key", Type: ui.BoolType, Value: true}}
	actual = convertTagsIntoMap(tags)
	assert.Empty(t, actual)

	tags = []ui.KeyValue{{Key: "key", Type: ui.Float64Type, Value: 0.8}}
	actual = convertTagsIntoMap(tags)
	assert.Equal(t, map[string]string{"key": "0.8"}, actual)
}

func TestRunTest(t *testing.T) {
	errFunc := func(service string, request *traceRequest) ([]*ui.Trace, error) {
		return nil, errors.New("test error")
	}
	successFunc := func(service string, request *traceRequest) ([]*ui.Trace, error) {
		return []*ui.Trace{}, nil
	}

	tests := []struct {
		request   *traceRequest
		f         testFunc
		shouldErr bool
	}{
		{&traceRequest{}, errFunc, true},
		{&traceRequest{Count: 1}, successFunc, true},
		{&traceRequest{Count: 0}, successFunc, false},
	}
	handler := &TraceHandler{}
	for _, test := range tests {
		err := handler.runTest("service", test.request, test.f, validateTracesWithCount)
		if test.shouldErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestValidateTracesWithCount(t *testing.T) {
	tests := []struct {
		expected *traceRequest
		actual   []*ui.Trace
		errMsg   string
	}{
		{&traceRequest{Count: 1}, nil, "expected 1 trace(s), got 0"},
		{&traceRequest{Count: 1}, []*ui.Trace{{}}, "expected 1 span, got 0"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "key", Type: ui.BoolType, Value: true},
					}},
				},
				},
			}, ""},
		{&traceRequest{Tags: map[string]string{"k": "v"}, Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "key", Type: ui.StringType, Value: "value"},
					}},
				},
				},
			}, "expected tags not found"},
		{&traceRequest{Tags: map[string]string{"key": "value"}, Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "key", Type: ui.StringType, Value: "value"},
					}},
				},
				},
			}, ""},
	}

	for _, test := range tests {
		err := validateTracesWithCount(test.expected, test.actual)
		if test.errMsg == "" {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, test.errMsg)
		}
	}
}

const (
	badOperation = "bad_op"
)

type testClientHandler struct{}

func (h testClientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, _ := ioutil.ReadAll(r.Body)

	var request traceRequest
	json.Unmarshal(body, &request)

	if request.Operation == badOperation {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func TestCreateTrace(t *testing.T) {
	server := httptest.NewServer(testClientHandler{})
	defer server.Close()

	handler := &TraceHandler{
		logger: zap.NewNop(),
		getClientURL: func(service string) string {
			return ""
		},
	}

	err := handler.createTrace("svc", &traceRequest{Operation: "op"})
	assert.Error(t, err)

	handler.getClientURL = func(service string) string {
		return server.URL
	}

	err = handler.createTrace("svc", &traceRequest{Operation: badOperation})
	assert.EqualError(t, err, "retrieved 400 status code from client service")

	err = handler.createTrace("svc", &traceRequest{Operation: "op"})
	assert.NoError(t, err)
}

func TestTraceHandlerGetTraces(t *testing.T) {
	query := &mocks.QueryService{}
	handler := NewTraceHandler(query, nil, zap.NewNop())
	handler.sleepDuration = time.Millisecond

	query.On("GetTraces", "crossdock-go", "op", mock.Anything).Return(nil, errors.New("queryError")).Times(10)
	traces := handler.getTraces("go", "op", nil)
	assert.Nil(t, traces)

	query.On("GetTraces", "crossdock-go", "op", mock.Anything).Return([]*ui.Trace{{TraceID: ui.TraceID(0)}}, nil)
	traces = handler.getTraces("go", "op", nil)
	assert.Len(t, traces, 1)
}

func TestEndToEndTest(t *testing.T) {
	query := &mocks.QueryService{}
	agent := &mocks.AgentService{}
	cT := &mocks.T{}
	handler := NewTraceHandler(query, agent, zap.NewNop())
	handler.sleepDuration = time.Millisecond

	cT.On("Param", "services").Return("go")
	cT.On("Errorf", mock.AnythingOfType("string"), mock.Anything)
	cT.On("Successf", mock.AnythingOfType("string"), mock.Anything)

	// Test with no http server
	handler.EndToEndTest(cT)
	cT.AssertNumberOfCalls(t, "Errorf", 1)

	server := httptest.NewServer(testClientHandler{})
	defer server.Close()
	handler.getClientURL = func(service string) string {
		return server.URL
	}

	// The query service fails to fetch traces
	query.On("GetTraces", "crossdock-go", mock.AnythingOfType("string"), mock.Anything).Return(nil, errors.New("queryError")).Times(10)

	handler.EndToEndTest(cT)
	cT.AssertNumberOfCalls(t, "Errorf", 2)

	trace := ui.Trace{
		TraceID: ui.TraceID(0),
		Spans:   []ui.Span{{Tags: []ui.KeyValue{{Key: "k", Value: "v", Type: ui.StringType}}}},
	}

	// The query service returns a trace
	query.On("GetTraces", "crossdock-go", mock.AnythingOfType("string"), mock.Anything).Return([]*ui.Trace{&trace}, nil)
	handler.getTags = func() map[string]string {
		return map[string]string{"k": "v"}
	}
	handler.EndToEndTest(cT)
	cT.AssertNumberOfCalls(t, "Successf", 1)
}
