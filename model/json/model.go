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

package json

import (
	"encoding/json"
	"io/ioutil"
)

// ReferenceType is the reference type of one span to another
type ReferenceType string

// TraceID is the shared trace ID of all spans in the trace.
type TraceID string

// SpanID is the id of a span
type SpanID string

// ProcessID is a hashed value of the Process struct that is unique within the trace.
type ProcessID string

// ValueType is the type of a value stored in KeyValue struct.
type ValueType string

const (
	// ChildOf means a span is the child of another span
	ChildOf ReferenceType = "CHILD_OF"
	// FollowsFrom means a span follows from another span
	FollowsFrom ReferenceType = "FOLLOWS_FROM"

	// StringType indicates a string value stored in KeyValue
	StringType ValueType = "string"
	// BoolType indicates a Boolean value stored in KeyValue
	BoolType ValueType = "bool"
	// Int64Type indicates a 64bit signed integer value stored in KeyValue
	Int64Type ValueType = "int64"
	// Float64Type indicates a 64bit float value stored in KeyValue
	Float64Type ValueType = "float64"
	// BinaryType indicates an arbitrary byte array stored in KeyValue
	BinaryType ValueType = "binary"
)

// Trace is a list of spans
type Trace struct {
	TraceID   TraceID               `json:"traceID"`
	Spans     []Span                `json:"spans"`
	Processes map[ProcessID]Process `json:"processes"`
	Warnings  []string              `json:"warnings,omitempty"`
}

// Span is a span denoting a piece of work in some infrastructure
type Span struct {
	TraceID       TraceID     `json:"traceID"`
	SpanID        SpanID      `json:"spanID"`
	Flags         uint32      `json:"flags,omitempty"`
	OperationName string      `json:"operationName"`
	References    []Reference `json:"references,omitempty"`
	StartTime     uint64      `json:"startTime"`
	Duration      uint64      `json:"duration"`
	Tags          []KeyValue  `json:"tags,omitempty"`
	Logs          []Log       `json:"logs,omitempty"`
	ProcessID     ProcessID   `json:"processID"`
	Warnings      []string    `json:"warnings,omitempty"`
}

// Reference is a reference from one span to another
type Reference struct {
	RefType ReferenceType `json:"refType"`
	TraceID TraceID       `json:"traceID"`
	SpanID  SpanID        `json:"spanID"`
}

// Process is the process emitting a set of spans
type Process struct {
	ServiceName string     `json:"serviceName"`
	Tags        []KeyValue `json:"tags,omitempty"`
}

// Log is a log emitted in a span
type Log struct {
	Timestamp uint64     `json:"timestamp"`
	Fields    []KeyValue `json:"fields,omitempty"`
}

// KeyValue is a a key-value pair with typed value.
type KeyValue struct {
	Key   string      `json:"key"`
	Type  ValueType   `json:"type,omitempty"`
	Value interface{} `json:"value"`
}

// DependencyLink shows dependencies between services
type DependencyLink struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	CallCount int64  `json:"callCount"`
}

// FromFile reads a Trace from a JSON file.
// Mostly this exists to have some code aside from struct delcaration,
// as otherwise code coverate is reported as 0%.
func FromFile(filename string) (*Trace, error) {
	in, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var trace Trace
	if err := json.Unmarshal(in, &trace); err != nil {
		return nil, err
	}
	return &trace, nil
}
