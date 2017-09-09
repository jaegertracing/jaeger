// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
