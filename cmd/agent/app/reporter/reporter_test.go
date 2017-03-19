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

package reporter

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"

	"github.com/uber/jaeger/cmd/agent/app/testutils"
)

func TestMultiReporter(t *testing.T) {
	r1, r2 := testutils.NewInMemoryReporter(), testutils.NewInMemoryReporter()
	r := NewMultiReporter(r1, r2)
	e1 := r.EmitZipkinBatch([]*zipkincore.Span{
		{},
	})
	e2 := r.EmitBatch(&jaeger.Batch{
		Spans: []*jaeger.Span{
			{},
		},
	})
	assert.NoError(t, e1)
	assert.NoError(t, e2)
	assert.Len(t, r1.ZipkinSpans(), 1)
	assert.Len(t, r1.Spans(), 1)
	assert.Len(t, r2.ZipkinSpans(), 1)
	assert.Len(t, r2.Spans(), 1)
}

func TestMultiReporterErrors(t *testing.T) {
	errMsg := "doh!"
	err := errors.New(errMsg)
	r1, r2 := alwaysFailReporter{err: err}, alwaysFailReporter{err: err}
	r := NewMultiReporter(r1, r2)
	e1 := r.EmitZipkinBatch([]*zipkincore.Span{
		{},
	})
	e2 := r.EmitBatch(&jaeger.Batch{
		Spans: []*jaeger.Span{
			{},
		},
	})
	assert.EqualError(t, e1, fmt.Sprintf("[%s, %s]", errMsg, errMsg))
	assert.EqualError(t, e2, fmt.Sprintf("[%s, %s]", errMsg, errMsg))
}

type alwaysFailReporter struct {
	err error
}

func (r alwaysFailReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	return r.err
}

func (r alwaysFailReporter) EmitBatch(batch *jaeger.Batch) error {
	return r.err
}
