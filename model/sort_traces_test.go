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
	tags = []KeyValue{
		{
			Key: "world",
		},
		{
			Key: "hello",
		},
	}
	logs = []Log{
		{
			Timestamp: time.Now(),
			Fields:    tags,
		},
		{
			Timestamp: time.Now().Add(-time.Hour),
			Fields:    tags,
		},
	}
)

func TestSortTraces(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
				Process: &Process{
					ServiceName: "hello",
					Tags:        tags,
				},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
				Process: &Process{
					ServiceName: "hello",
					Tags:        tags,
				},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	assert.NoError(t, SortTraces(t1, t2))
}

func TestSortTraces_DifferentNumberOfSpansError(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
			},
		},
	}
	assert.EqualError(t, SortTraces(t1, t2), "traces have different number of spans")
}

func TestSortTraces_DifferentLengthOfTagsError(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    append(tags, KeyValue{Key: "different"}),
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	assert.EqualError(t, SortTraces(t1, t2), "tags have different length")
}

func TestSortTraces_DifferentLengthOfLogsError(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    append(logs, Log{Timestamp: time.Now().Add(-time.Minute)}),
			},
		},
	}
	assert.EqualError(t, SortTraces(t1, t2), "logs have different length")
}

func TestSortTraces_ProcessNilError(t *testing.T) {
	t1 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
				Process: &Process{},
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	t2 := &Trace{
		Spans: []*Span{
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(2),
				Tags:    tags,
			},
			{
				TraceID: TraceID{Low: 1},
				SpanID:  SpanID(1),
				Logs:    logs,
			},
		},
	}
	assert.EqualError(t, SortTraces(t1, t2), "process does not match")
}
