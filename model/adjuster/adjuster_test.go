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

package adjuster_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/model/adjuster"
)

func TestSequence(t *testing.T) {
	span := model.Span{SpanID: 123}
	trace := model.Trace{Spans: []*model.Span{&span}}

	adj := adjuster.Func(func(trace model.Trace) (model.Trace, error) {
		trace.Spans[0].SpanID++
		return trace, nil
	})

	seq := adjuster.Sequence(adj, adj)
	adjTrace, err := seq.Adjust(trace)
	assert.NoError(t, err)
	assert.EqualValues(t, 125, adjTrace.Spans[0].SpanID)
}
