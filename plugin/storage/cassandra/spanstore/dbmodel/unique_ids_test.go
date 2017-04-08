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

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestGetUniqueTraceIDs(t *testing.T) {
	const someOperationName = "operation-name"
	firstTraceID := TraceIDFromDomain(model.TraceID{High: 1, Low: 1})
	secondTraceID := TraceIDFromDomain(model.TraceID{High: 2, Low: 2})
	testInput := []Span{
		{
			TraceID:       firstTraceID,
			SpanID:        1,
			OperationName: someOperationName,
		},
		{
			TraceID:       firstTraceID,
			SpanID:        2,
			OperationName: someOperationName,
		},
		{
			TraceID:       secondTraceID,
			SpanID:        3,
			OperationName: someOperationName,
		},
	}
	uniqueTraces := GetUniqueTraceIDs(testInput)
	assert.Len(t, uniqueTraces, 2)
	_, ok := uniqueTraces[firstTraceID]
	assert.True(t, ok)
	_, ok = uniqueTraces[secondTraceID]
	assert.True(t, ok)
}

func TestGetIntersectedTraceIDs(t *testing.T) {
	firstTraceID := TraceIDFromDomain(model.TraceID{High: 1, Low: 1})
	secondTraceID := TraceIDFromDomain(model.TraceID{High: 2, Low: 2})
	listOfUniqueTraceIDs := []UniqueTraceIDs{
		{
			firstTraceID:  struct{}{},
			secondTraceID: struct{}{},
		},
		{
			firstTraceID:  struct{}{},
			secondTraceID: struct{}{},
		},
		{
			firstTraceID: struct{}{},
		},
	}
	expected := UniqueTraceIDs{
		firstTraceID: struct{}{},
	}
	actual := IntersectTraceIDs(listOfUniqueTraceIDs)
	assert.EqualValues(t, expected, actual)
}

func TestAdd(t *testing.T) {
	u := UniqueTraceIDs{}
	someID := TraceIDFromDomain(model.TraceID{High: 1, Low: 1})
	u.Add(someID)
	assert.Contains(t, u, someID)
}

func TestToList(t *testing.T) {
	u := UniqueTraceIDs{}
	someID := TraceIDFromDomain(model.TraceID{High: 1, Low: 1})
	u.Add(someID)
	l := u.ToList()
	assert.Len(t, l, 1)
	assert.Equal(t, someID, l[0])
}

func TestFromList(t *testing.T) {
	someID := TraceIDFromDomain(model.TraceID{High: 1, Low: 1})
	traceIDList := []TraceID{
		someID,
	}
	uniqueTraceIDs := UniqueTraceIDsFromList(traceIDList)
	assert.Len(t, uniqueTraceIDs, 1)
	assert.Contains(t, uniqueTraceIDs, someID)
}
