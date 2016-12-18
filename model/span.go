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

package model

// TraceID is a random 128bit identifier for a trace
type TraceID struct {
	Low  uint64 `json:"lo"`
	High uint64 `json:"hi"`
}

// SpanID is a random 64bit identifier for a span
type SpanID uint64

// Span represents a unit of work in an application, such as an RPC, a database call, etc.
type Span struct {
	TraceID       TraceID   `json:"traceId"`
	SpanID        SpanID    `json:"spanId"`
	ParentSpanID  SpanID    `json:"parentSpanId"`
	OperationName string    `json:"operationName"`
	References    []SpanRef `json:"references,omitempty"`
	Flags         uint32    `json:"flags"`
	StartTime     uint64    `json:"startTime"`
	Duration      uint64    `json:"duration"`
	Tags          KeyValues `json:"tags,omitempty"`
	Logs          []Log     `json:"logs,omitempty"`
	Process       *Process  `json:"process"`
}
