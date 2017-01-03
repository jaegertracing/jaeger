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

package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestTraceFindSpanByID(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{SpanID: model.SpanID(1), OperationName: "x"},
			{SpanID: model.SpanID(2), OperationName: "y"},
			{SpanID: model.SpanID(1), OperationName: "z"}, // same span ID
		},
	}
	s1 := trace.FindSpanByID(model.SpanID(1))
	assert.NotNil(t, s1)
	assert.Equal(t, "x", s1.OperationName)
	s2 := trace.FindSpanByID(model.SpanID(2))
	assert.NotNil(t, s2)
	assert.Equal(t, "y", s2.OperationName)
	s3 := trace.FindSpanByID(model.SpanID(3))
	assert.Nil(t, s3)
}
