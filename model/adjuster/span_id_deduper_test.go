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

package adjuster

import (
	"testing"

	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

const (
	clientSpanID  = model.SpanID(1)
	anotherSpanID = model.SpanID(11)
)

func newTrace() *model.Trace {
	return &model.Trace{
		Spans: []*model.Span{
			{
				// client span
				SpanID: clientSpanID,
				Tags: model.KeyValues{
					// span.kind = client
					model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
				},
			},
			{
				// server span
				SpanID: clientSpanID, // shared span ID
				Tags: model.KeyValues{
					// span.kind = server
					model.String(string(ext.SpanKind), string(ext.SpanKindRPCServerEnum)),
				},
			},
			{
				// some other span, child of server span
				SpanID:       anotherSpanID,
				ParentSpanID: clientSpanID,
			},
		},
	}
}

func TestSpanIDDeduperTriggered(t *testing.T) {
	trace := newTrace()
	deduper := SpanIDDeduper()
	trace, err := deduper.Adjust(trace)
	assert.NoError(t, err)

	clientSpan := trace.Spans[0]
	assert.Equal(t, clientSpanID, clientSpan.SpanID, "client span ID should not change")

	serverSpan := trace.Spans[1]
	assert.Equal(t, clientSpanID+1, serverSpan.SpanID, "server span ID should be reassigned")
	assert.Equal(t, clientSpan.SpanID, serverSpan.ParentSpanID, "client span should be server span's parent")

	thirdSpan := trace.Spans[2]
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID, "3rd span ID should not change")
	assert.Equal(t, serverSpan.SpanID, thirdSpan.ParentSpanID, "server span should be 3rd span's parent")
}

func TestSpanIDDeduperNotTriggered(t *testing.T) {
	trace := newTrace()
	trace.Spans = trace.Spans[1:] // remove client span

	deduper := SpanIDDeduper()
	trace, err := deduper.Adjust(trace)
	assert.NoError(t, err)

	serverSpanID := clientSpanID // for better readability
	serverSpan := trace.Spans[0]
	assert.Equal(t, serverSpanID, serverSpan.SpanID, "server span ID should be unchanged")

	thirdSpan := trace.Spans[1]
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID, "3rd span ID should not change")
	assert.Equal(t, serverSpan.SpanID, thirdSpan.ParentSpanID, "server span should be 3rd span's parent")
}

func TestSpanIDDeduperError(t *testing.T) {
	trace := newTrace()

	maxID := int64(-1)
	assert.Equal(t, maxSpanID, model.SpanID(maxID), "maxSpanID must be 2^64-1")

	deduper := &spanIDDeduper{trace: trace}
	deduper.groupSpansByID()
	deduper.maxUsedID = maxSpanID - 1
	deduper.dedupeSpanIDs()
	if assert.Len(t, trace.Spans[1].Warnings, 1) {
		assert.Equal(t, trace.Spans[1].Warnings[0], "cannot assign unique span ID, too many spans in the trace")
	}
}
