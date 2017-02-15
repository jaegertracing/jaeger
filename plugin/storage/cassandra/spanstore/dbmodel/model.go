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
	"bytes"
	"encoding/binary"

	"github.com/uber/jaeger/model"
)

// TraceID is a serializable form of model.TraceID
type TraceID [16]byte

// Span is the database representation of a span.
type Span struct {
	TraceID       TraceID
	SpanID        int64
	ParentID      int64
	OperationName string
	Flags         int32
	StartTime     int64
	Duration      int64
	Tags          []KeyValue
	Logs          []Log
	Refs          []SpanRef
	Process       Process
	ServiceName   string
	SpanHash      int64
}

// KeyValue is the UDT representation of a Jaeger KeyValue.
type KeyValue struct {
	Key          string  `cql:"key"`
	ValueType    string  `cql:"value_type"`
	ValueString  string  `cql:"value_string"`
	ValueBool    bool    `cql:"value_bool"`
	ValueInt64   int64   `cql:"value_long"`   // using more natural column name for Cassandra
	ValueFloat64 float64 `cql:"value_double"` // using more natural column name for Cassandra
	ValueBinary  []byte  `cql:"value_binary"`
}

// Log is the UDT representation of a Jaeger Log.
type Log struct {
	Timestamp int64      `cql:"ts"`
	Fields    []KeyValue `cql:"fields"`
}

// SpanRef is the UDT representation of a Jaeger Span Reference.
type SpanRef struct {
	RefType string  `cql:"ref_type"`
	TraceID TraceID `cql:"trace_id"`
	SpanID  int64   `cql:"span_id"`
}

// Process is the UDT representation of a Jaeger Process.
type Process struct {
	ServiceName string     `cql:"service_name"`
	Tags        []KeyValue `cql:"tags"`
}

// TagInsertion contains the items necessary to insert a tag for a given span
type TagInsertion struct {
	ServiceName string
	TagKey      string
	TagValue    string
}

func (t TagInsertion) String() string {
	const uniqueTagDelimiter = ":"
	var buffer bytes.Buffer
	buffer.WriteString(t.ServiceName)
	buffer.WriteString(uniqueTagDelimiter)
	buffer.WriteString(t.TagKey)
	buffer.WriteString(uniqueTagDelimiter)
	buffer.WriteString(t.TagValue)
	return buffer.String()
}

// TraceIDFromDomain converts domain TraceID into serializable DB representation.
func TraceIDFromDomain(traceID model.TraceID) TraceID {
	dbTraceID := TraceID{}
	binary.BigEndian.PutUint64(dbTraceID[:8], uint64(traceID.High))
	binary.BigEndian.PutUint64(dbTraceID[8:], uint64(traceID.Low))
	return dbTraceID
}

// ToDomain converts trace ID from db-serializable form to domain TradeID
func (dbTraceID TraceID) ToDomain() model.TraceID {
	traceIDHigh := binary.BigEndian.Uint64(dbTraceID[:8])
	traceIDLow := binary.BigEndian.Uint64(dbTraceID[8:])
	return model.TraceID{High: traceIDHigh, Low: traceIDLow}
}

// String returns hex string representation of the trace ID.
func (dbTraceID TraceID) String() string {
	return dbTraceID.ToDomain().String()
}
