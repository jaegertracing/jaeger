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

package spanstore

import (
	"errors"
	"time"

	"github.com/uber/jaeger/model"
)

// Writer writes spans to storage.
type Writer interface {
	WriteSpan(span *model.Span) error
}

var (
	// ErrTraceNotFound is returned by Reader's GetTrace if no data is found for given trace ID.
	ErrTraceNotFound = errors.New("trace not found")
)

// Reader finds and loads traces and other data from storage.
type Reader interface {
	GetTrace(traceID model.TraceID) (*model.Trace, error)
	GetServices() ([]string, error)
	GetOperations(service string) ([]string, error)
	FindTraces(query *TraceQueryParameters) ([]*model.Trace, error)
}

// TraceQueryParameters contains parameters of a trace query.
type TraceQueryParameters struct {
	ServiceName   string
	OperationName string
	Tags          map[string]string
	StartTimeMin  time.Time
	StartTimeMax  time.Time
	DurationMin   time.Duration
	DurationMax   time.Duration
	NumTraces     int
}
