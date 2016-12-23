// Copyright (c) 2016 Uber Technologies, Inc.
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

package model_test

import (
	"testing"

	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"

	"encoding/json"

	"github.com/uber/jaeger/model"
)

type TraceIDContainer struct {
	TraceID model.TraceID `json:"id"`
}

func TestTraceIDMarshalText(t *testing.T) {
	testCases := []struct {
		hi, lo uint64
		out    string
	}{
		{lo: 1, out: `{"id":"1"}`},
		{lo: 15, out: `{"id":"f"}`},
		{lo: 31, out: `{"id":"1f"}`},
		{lo: 257, out: `{"id":"101"}`},
		{hi: 1, lo: 1, out: `{"id":"10000000000000001"}`},
		{hi: 257, lo: 1, out: `{"id":"1010000000000000001"}`},
	}
	for _, testCase := range testCases {
		c := TraceIDContainer{TraceID: model.TraceID{High: testCase.hi, Low: testCase.lo}}
		out, err := json.Marshal(&c)
		if assert.NoError(t, err) {
			assert.Equal(t, testCase.out, string(out))
		}
	}
}

func TestTraceIDUnmarshalText(t *testing.T) {
	testCases := []struct {
		in     string
		hi, lo uint64
		err    bool
	}{
		{lo: 1, in: `{"id":"1"}`},
		{lo: 15, in: `{"id":"f"}`},
		{lo: 31, in: `{"id":"1f"}`},
		{lo: 257, in: `{"id":"101"}`},
		{hi: 1, lo: 1, in: `{"id":"10000000000000001"}`},
		{hi: 257, lo: 1, in: `{"id":"1010000000000000001"}`},
		{err: true, in: `{"id":""}`},
		{err: true, in: `{"id":"x"}`},
		{err: true, in: `{"id":"x0000000000000001"}`},
		{err: true, in: `{"id":"1x000000000000001"}`},
		{err: true, in: `{"id":"10123456789abcdef0123456789abcdef"}`},
	}
	for _, testCase := range testCases {
		var c TraceIDContainer
		err := json.Unmarshal([]byte(testCase.in), &c)
		if testCase.err {
			assert.Error(t, err)
		} else {
			if assert.NoError(t, err) {
				assert.Equal(t, testCase.hi, c.TraceID.High)
				assert.Equal(t, testCase.lo, c.TraceID.Low)
			}
		}
	}
}

type SpanIDContainer struct {
	SpanID model.SpanID `json:"id"`
}

func TestSpanIDMarshalText(t *testing.T) {
	max := int64(-1)
	testCases := []struct {
		id  uint64
		out string
	}{
		{id: 1, out: `{"id":"1"}`},
		{id: 15, out: `{"id":"f"}`},
		{id: 31, out: `{"id":"1f"}`},
		{id: 257, out: `{"id":"101"}`},
		{id: uint64(max), out: `{"id":"ffffffffffffffff"}`},
	}
	for _, testCase := range testCases {
		c := SpanIDContainer{SpanID: model.SpanID(testCase.id)}
		out, err := json.Marshal(&c)
		if assert.NoError(t, err) {
			assert.Equal(t, testCase.out, string(out))
		}
	}
}

func TestSpanIDUnmarshalText(t *testing.T) {
	testCases := []struct {
		in  string
		id  uint64
		err bool
	}{
		{id: 1, in: `{"id":"1"}`},
		{id: 15, in: `{"id":"f"}`},
		{id: 31, in: `{"id":"1f"}`},
		{id: 257, in: `{"id":"101"}`},
		{err: true, in: `{"id":""}`},
		{err: true, in: `{"id":"x"}`},
		{err: true, in: `{"id":"x123"}`},
		{err: true, in: `{"id":"10123456789abcdef"}`},
	}
	for _, testCase := range testCases {
		var c SpanIDContainer
		err := json.Unmarshal([]byte(testCase.in), &c)
		if testCase.err {
			assert.Error(t, err)
		} else {
			if assert.NoError(t, err) {
				assert.Equal(t, testCase.id, uint64(c.SpanID))
			}
		}
	}
}

func TestIsRPCClientServer(t *testing.T) {
	span1 := &model.Span{
		Tags: model.KeyValues{
			model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
		},
	}
	assert.True(t, span1.IsRPCClient())
	assert.False(t, span1.IsRPCServer())
	span2 := &model.Span{}
	assert.False(t, span2.IsRPCClient())
	assert.False(t, span2.IsRPCServer())
}
