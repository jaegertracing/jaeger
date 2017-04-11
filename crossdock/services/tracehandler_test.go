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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-client-go"
	ui "github.com/uber/jaeger/model/json"
)

func TestCreateTraceRequest(t *testing.T) {
	req := createTraceRequest(jaeger.SamplerTypeConst, "op", 23)
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

func TestValidateAdaptiveSamplingTraces(t *testing.T) {
	tests := []struct {
		expected *traceRequest
		actual   []*ui.Trace
		errMsg   string
	}{
		{&traceRequest{Count: 1}, []*ui.Trace{{}}, "expected 1 span, got 0"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "key", Type: ui.BoolType, Value: true},
					}},
				},
				},
			}, "sampler.param and sampler.type tags not found"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "sampler.param", Type: ui.StringType, Value: "0.0203"},
					}},
				},
				},
			}, "sampler.param and sampler.type tags not found"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "sampler.param", Type: ui.StringType, Value: "not_float"},
						{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
					}},
				},
				},
			}, "sampler.param tag value is not a float: not_float"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "sampler.param", Type: ui.StringType, Value: "0.003"},
						{Key: "sampler.type", Type: ui.StringType, Value: "const"},
					}},
				},
				},
			}, "sampler.type tag value should be 'probabilistic'"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "sampler.param", Type: ui.StringType, Value: "0.001"},
						{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
					}},
				},
				},
			}, "adaptive sampling probability not used"},
		{&traceRequest{Count: 1},
			[]*ui.Trace{
				{Spans: []ui.Span{
					{Tags: []ui.KeyValue{
						{Key: "sampler.param", Type: ui.StringType, Value: "0.02314"},
						{Key: "sampler.type", Type: ui.StringType, Value: "probabilistic"},
					}},
				},
				},
			}, ""},
	}
	for _, test := range tests {
		err := validateAdaptiveSamplingTraces(test.expected, test.actual)
		if test.errMsg == "" {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, test.errMsg)
		}
	}
}
