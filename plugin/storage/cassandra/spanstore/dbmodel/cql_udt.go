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
	"errors"
	"fmt"

	"github.com/gocql/gocql"
)

// ErrTraceIDWrongLength is an error that occurs when cassandra has a TraceID that's not 128 bits long
var ErrTraceIDWrongLength = errors.New("TraceID is not a 128bit integer")

// MarshalCQL handles marshaling DBTraceID (e.g. in SpanRef)
func (t TraceID) MarshalCQL(info gocql.TypeInfo) ([]byte, error) {
	return t[:], nil
}

// UnmarshalCQL handles unmarshaling DBTraceID (e.g. in SpanRef)
func (t *TraceID) UnmarshalCQL(info gocql.TypeInfo, data []byte) error {
	if len(data) != 16 {
		return ErrTraceIDWrongLength
	}
	copy(t[:], data)
	return nil
}

// MarshalUDT handles marshalling a Tag.
func (t *KeyValue) MarshalUDT(name string, info gocql.TypeInfo) ([]byte, error) {
	switch name {
	case "key":
		return gocql.Marshal(info, t.Key)
	case "value_type":
		return gocql.Marshal(info, t.ValueType)
	case "value_string":
		return gocql.Marshal(info, t.ValueString)
	case "value_bool":
		return gocql.Marshal(info, t.ValueBool)
	case "value_long":
		return gocql.Marshal(info, t.ValueInt64)
	case "value_double":
		return gocql.Marshal(info, t.ValueFloat64)
	case "value_binary":
		return gocql.Marshal(info, t.ValueBinary)
	default:
		return nil, fmt.Errorf("unknown column for position: %q", name)
	}
}

// UnmarshalUDT handles unmarshalling a Tag.
func (t *KeyValue) UnmarshalUDT(name string, info gocql.TypeInfo, data []byte) error {
	switch name {
	case "key":
		return gocql.Unmarshal(info, data, &t.Key)
	case "value_type":
		return gocql.Unmarshal(info, data, &t.ValueType)
	case "value_string":
		return gocql.Unmarshal(info, data, &t.ValueString)
	case "value_bool":
		return gocql.Unmarshal(info, data, &t.ValueBool)
	case "value_long":
		return gocql.Unmarshal(info, data, &t.ValueInt64)
	case "value_double":
		return gocql.Unmarshal(info, data, &t.ValueFloat64)
	case "value_binary":
		return gocql.Unmarshal(info, data, &t.ValueBinary)
	default:
		return fmt.Errorf("unknown column for position: %q", name)
	}
}

// MarshalUDT handles marshalling a Log.
func (l *Log) MarshalUDT(name string, info gocql.TypeInfo) ([]byte, error) {
	switch name {
	case "ts":
		return gocql.Marshal(info, l.Timestamp)
	case "fields":
		return gocql.Marshal(info, l.Fields)
	default:
		return nil, fmt.Errorf("unknown column for position: %q", name)
	}
}

// UnmarshalUDT handles unmarshalling a Log.
func (l *Log) UnmarshalUDT(name string, info gocql.TypeInfo, data []byte) error {
	switch name {
	case "ts":
		return gocql.Unmarshal(info, data, &l.Timestamp)
	case "fields":
		return gocql.Unmarshal(info, data, &l.Fields)
	default:
		return fmt.Errorf("unknown column for position: %q", name)
	}
}

// MarshalUDT handles marshalling a SpanRef.
func (s *SpanRef) MarshalUDT(name string, info gocql.TypeInfo) ([]byte, error) {
	switch name {
	case "ref_type":
		return gocql.Marshal(info, s.RefType)
	case "trace_id":
		return gocql.Marshal(info, s.TraceID)
	case "span_id":
		return gocql.Marshal(info, s.SpanID)
	default:
		return nil, fmt.Errorf("unknown column for position: %q", name)
	}
}

// UnmarshalUDT handles unmarshalling a SpanRef.
func (s *SpanRef) UnmarshalUDT(name string, info gocql.TypeInfo, data []byte) error {
	switch name {
	case "ref_type":
		return gocql.Unmarshal(info, data, &s.RefType)
	case "trace_id":
		return gocql.Unmarshal(info, data, &s.TraceID)
	case "span_id":
		return gocql.Unmarshal(info, data, &s.SpanID)
	default:
		return fmt.Errorf("unknown column for position: %q", name)
	}
}

// MarshalUDT handles marshalling a Process.
func (p *Process) MarshalUDT(name string, info gocql.TypeInfo) ([]byte, error) {
	switch name {
	case "service_name":
		return gocql.Marshal(info, p.ServiceName)
	case "tags":
		return gocql.Marshal(info, p.Tags)
	default:
		return nil, fmt.Errorf("unknown column for position: %q", name)
	}
}

// UnmarshalUDT handles unmarshalling a Process.
func (p *Process) UnmarshalUDT(name string, info gocql.TypeInfo, data []byte) error {
	switch name {
	case "service_name":
		return gocql.Unmarshal(info, data, &p.ServiceName)
	case "tags":
		return gocql.Unmarshal(info, data, &p.Tags)
	default:
		return fmt.Errorf("unknown column for position: %q", name)
	}
}
