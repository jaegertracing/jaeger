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

package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-client-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/crossdock/services/mocks"
	ui "github.com/jaegertracing/jaeger/model/json"
)

var (
	testTrace = ui.Trace{
		TraceID: ui.TraceID("0"),
		Spans:   []ui.Span{{Tags: []ui.KeyValue{{Key: "k", Value: "v", Type: ui.StringType}}}},
	}
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
		request   traceRequest
		f         testFunc
		shouldErr bool
	}{
		{
			request:   traceRequest{},
			f:         errFunc,
			shouldErr: true,
		},
		{
			request:   traceRequest{Count: 1},
			f:         successFunc,
			shouldErr: true,
		},
		{
			request:   traceRequest{Count: 0},
			f:         successFunc,
			shouldErr: false,
		},
	}
	handler := &TraceHandler{}
	for _, test := range tests {
		err := handler.runTest("service", &test.request, test.f, validateTracesWithCount)
		if test.shouldErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestValidateTracesWithCount(t *testing.T) {
	tests := []struct {
		expected traceRequest
		actual   []*ui.Trace
		errMsg   string
	}{
		{
			expected: traceRequest{Count: 1},
			errMsg:   "expected 1 trace(s), got 0",
		},
		{
			expected: traceRequest{Count: 1},
			actual:   []*ui.Trace{{}},
			errMsg:   "expected 1 span, got 0",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "key", Type: ui.BoolType, Value: true},
							},
						},
					},
				},
			},
		},
		{
			expected: traceRequest{Tags: map[string]string{"k": "v"}, Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "key", Type: ui.StringType, Value: "value"},
							},
						},
					},
				},
			},
			errMsg: "expected tags not found",
		},
		{
			expected: traceRequest{Tags: map[string]string{"key": "value"}, Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "key", Type: ui.StringType, Value: "value"},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		err := validateTracesWithCount(&test.expected, test.actual)
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

type testClientHandler struct {
	sync.RWMutex
	callCount int64
}

func (h *testClientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, _ := ioutil.ReadAll(r.Body)

	var request traceRequest
	json.Unmarshal(body, &request)

	if request.Operation == badOperation {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	h.Lock()
	defer h.Unlock()
	h.callCount++
}

func (h *testClientHandler) CallCount() int64 {
	h.RLock()
	defer h.RUnlock()
	return h.callCount
}

func TestCreateTrace(t *testing.T) {
	server := httptest.NewServer(&testClientHandler{})
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
	handler.getTracesSleepDuration = time.Millisecond

	query.On("GetTraces", "crossdock-go", "op", mock.Anything).Return(nil, errors.New("queryError")).Times(10)
	traces := handler.getTraces("go", "op", nil)
	assert.Nil(t, traces)

	query.On("GetTraces", "crossdock-go", "op", mock.Anything).Return([]*ui.Trace{{TraceID: ui.TraceID("0")}}, nil)
	traces = handler.getTraces("go", "op", nil)
	assert.Len(t, traces, 1)
}

func TestCreateTracesLoop(t *testing.T) {
	h := &testClientHandler{}
	server := httptest.NewServer(h)
	defer server.Close()

	handler := &TraceHandler{
		logger:                   zap.NewNop(),
		createTracesLoopInterval: time.Millisecond,
		getClientURL: func(service string) string {
			return server.URL
		},
	}

	stop := make(chan struct{})
	go handler.createTracesLoop("svc", traceRequest{Operation: "op"}, stop)
	defer close(stop)

	for i := 0; i < 100; i++ {
		if h.CallCount() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	assert.True(t, h.CallCount() > 0)
}

func TestValidateAdaptiveSamplingTraces(t *testing.T) {
	tests := []struct {
		expected traceRequest
		actual   []*ui.Trace
		errMsg   string
	}{
		{
			expected: traceRequest{Count: 1},
			actual:   []*ui.Trace{{}},
			errMsg:   "expected 1 span, got 0",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "key", Type: ui.BoolType, Value: true},
							},
						},
					},
				},
			},
			errMsg: "sampler.param and sampler.type tags not found",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "sampler.param", Type: ui.StringType, Value: "0.0203"},
							},
						},
					},
				},
			},
			errMsg: "sampler.param and sampler.type tags not found",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "sampler.param", Type: ui.StringType, Value: "not_float"},
								{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
							},
						},
					},
				},
			},
			errMsg: "sampler.param tag value is not a float: not_float",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "sampler.param", Type: ui.StringType, Value: "0.003"},
								{Key: "sampler.type", Type: ui.StringType, Value: "const"},
							},
						},
					},
				},
			},
			errMsg: "sampler.type tag value should be 'probabilistic'",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "sampler.param", Type: ui.StringType, Value: "0.001"},
								{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
							},
						},
					},
				},
			},
			errMsg: "adaptive sampling probability not used",
		},
		{
			expected: traceRequest{Count: 1},
			actual: []*ui.Trace{
				{
					Spans: []ui.Span{
						{
							Tags: []ui.KeyValue{
								{Key: "sampler.param", Type: ui.StringType, Value: "0.02314"},
								{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
							},
						},
					},
				},
			},
			errMsg: "",
		},
	}
	for _, test := range tests {
		err := validateAdaptiveSamplingTraces(&test.expected, test.actual)
		if test.errMsg == "" {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, test.errMsg)
		}
	}
}

func TestAdaptiveSamplingTestInternal(t *testing.T) {
	server := httptest.NewServer(&testClientHandler{})
	defer server.Close()

	tests := []struct {
		samplingRate       float64
		getSamplingRateErr error
		shouldGetTracesErr bool
		errMsg             string
	}{
		{
			getSamplingRateErr: errors.New("http error"),
			errMsg:             "could not retrieve sampling rate from agent: http error",
		},
		{
			samplingRate: defaultProbabilities[0],
			errMsg:       "failed to retrieve adaptive sampling rate",
		},
		{
			samplingRate:       0.22,
			shouldGetTracesErr: true,
			errMsg:             "could not retrieve traces from query service",
		},
		{
			samplingRate:       0.22,
			shouldGetTracesErr: false,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			query := &mocks.QueryService{}
			agent := &mocks.AgentService{}

			handler := &TraceHandler{
				agent:  agent,
				query:  query,
				logger: zap.NewNop(),
				getClientURL: func(service string) string {
					return server.URL
				},
				createTracesLoopInterval:              time.Second,
				getSamplingRateInterval:               time.Millisecond,
				clientSamplingStrategyRefreshInterval: time.Millisecond,
				getTracesSleepDuration:                time.Millisecond,
			}

			agent.On("GetSamplingRate", "svc", "op").Return(test.samplingRate, test.getSamplingRateErr)
			if test.shouldGetTracesErr {
				query.On("GetTraces", "crossdock-svc", "op", mock.Anything).Return(nil, errors.New("queryError")).Times(10)
			} else {
				query.On("GetTraces", "crossdock-svc", "op", mock.Anything).Return([]*ui.Trace{&testTrace}, nil)
			}

			_, err := handler.adaptiveSamplingTest("svc", &traceRequest{Operation: "op"})
			if test.errMsg != "" {
				assert.EqualError(t, err, test.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEndToEndTest(t *testing.T) {
	query := &mocks.QueryService{}
	agent := &mocks.AgentService{}
	cT := &mocks.T{}
	handler := NewTraceHandler(query, agent, zap.NewNop())
	handler.getTracesSleepDuration = time.Millisecond

	cT.On("Param", "services").Return("go")
	cT.On("Errorf", mock.AnythingOfType("string"), mock.Anything)
	cT.On("Successf", mock.AnythingOfType("string"), mock.Anything)

	// Test with no http server
	handler.EndToEndTest(cT)
	cT.AssertNumberOfCalls(t, "Errorf", 1)

	server := httptest.NewServer(&testClientHandler{})
	defer server.Close()
	handler.getClientURL = func(service string) string {
		return server.URL
	}

	// The query service fails to fetch traces
	query.On("GetTraces", "crossdock-go", mock.AnythingOfType("string"), mock.Anything).Return(nil, errors.New("queryError")).Times(10)

	handler.EndToEndTest(cT)
	cT.AssertNumberOfCalls(t, "Errorf", 2)

	// The query service returns a trace
	query.On("GetTraces", "crossdock-go", mock.AnythingOfType("string"), mock.Anything).Return([]*ui.Trace{&testTrace}, nil)
	handler.getTags = func() map[string]string {
		return map[string]string{"k": "v"}
	}
	handler.EndToEndTest(cT)
	cT.AssertNumberOfCalls(t, "Successf", 1)
}

func TestAdaptiveSamplingTest(t *testing.T) {
	server := httptest.NewServer(&testClientHandler{})
	defer server.Close()

	query := &mocks.QueryService{}
	agent := &mocks.AgentService{}
	cT := &mocks.T{}
	handler := &TraceHandler{
		agent:  agent,
		query:  query,
		logger: zap.NewNop(),
		getClientURL: func(service string) string {
			return server.URL
		},
		getTags: func() map[string]string {
			return map[string]string{}
		},
		createTracesLoopInterval:              time.Second,
		getSamplingRateInterval:               time.Millisecond,
		clientSamplingStrategyRefreshInterval: time.Millisecond,
		getTracesSleepDuration:                time.Millisecond,
	}

	cT.On("Param", "services").Return("go")
	cT.On("Errorf", mock.AnythingOfType("string"), mock.Anything)
	cT.On("Successf", mock.AnythingOfType("string"), mock.Anything)

	// Test with Agent only returning defaultProbabilities
	agent.On("GetSamplingRate", "go", mock.AnythingOfType("string")).Return(defaultProbabilities[0], nil)
	handler.AdaptiveSamplingTest(cT)
	cT.AssertNumberOfCalls(t, "Errorf", 1)

	adaptiveSamplingTrace := ui.Trace{
		Spans: []ui.Span{
			{
				Tags: []ui.KeyValue{
					{Key: "sampler.param", Type: ui.StringType, Value: "0.02314"},
					{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
					{Key: "adaptive", Type: ui.StringType, Value: "sampling"},
				},
			},
		},
	}

	agent = &mocks.AgentService{}
	handler.agent = agent
	agent.On("GetSamplingRate", "go", mock.AnythingOfType("string")).Return(0.222, nil)
	// The query service returns an adaptive sampled trace
	query.On("GetTraces", "crossdock-go", mock.AnythingOfType("string"), mock.Anything).Return([]*ui.Trace{&adaptiveSamplingTrace}, nil)
	handler.AdaptiveSamplingTest(cT)
	cT.AssertNumberOfCalls(t, "Successf", 1)
}
