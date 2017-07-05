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

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	currTime = time.Now()
)

func TestSortTraces(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    []KeyValue{{Key: "world"}, {Key: "hello"}},
				Process: &Process{
					ServiceName: "hello",
					Tags:        []KeyValue{{Key: "hello"}, {Key: "world"}},
				},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs: []Log{
					{
						Timestamp: currTime,
						Fields:    []KeyValue{{Key: "world"}, {Key: "hello"}},
					},
					{
						Timestamp: currTime.Add(-time.Hour),
						Fields:    []KeyValue{{Key: "hello"}, {Key: "world"}},
					},
				},
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    []KeyValue{{Key: "world"}, {Key: "hello"}},
				Process: &Process{
					ServiceName: "hello",
					Tags:        []KeyValue{{Key: "hello"}, {Key: "world"}},
				},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs: []Log{
					{
						Timestamp: currTime.Add(-time.Hour),
						Fields:    []KeyValue{{Key: "world"}, {Key: "hello"}},
					},
					{
						Timestamp: currTime,
						Fields:    []KeyValue{{Key: "hello"}, {Key: "world"}},
					},
				},
			},
		},
	}
	SortTrace(t1)
	SortTrace(t2)
	assert.EqualValues(t, t1, t2)
}
