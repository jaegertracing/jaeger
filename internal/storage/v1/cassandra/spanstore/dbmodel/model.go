// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	childOf     = "child-of"
	followsFrom = "follows-from"

	stringType  = "string"
	boolType    = "bool"
	int64Type   = "int64"
	float64Type = "float64"
	binaryType  = "binary"
)

// TraceID is a serializable form of model.TraceID
type TraceID [16]byte

// Span is the database representation of a span.
type Span struct {
	TraceID       TraceID
	SpanID        int64
	ParentID      int64 // deprecated
	OperationName string
	Flags         int32
	StartTime     int64 // microseconds since epoch
	Duration      int64 // microseconds
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

func (kv *KeyValue) Compare(that any) int {
	if that == nil {
		if kv == nil {
			return 0
		}
		return 1
	}
	that1, ok := that.(*KeyValue)
	if !ok {
		that2, ok := that.(KeyValue)
		if !ok {
			return 1
		}
		that1 = &that2
	}
	if that1 == nil {
		if kv == nil {
			return 0
		}
		return 1
	} else if kv == nil {
		return -1
	}
	if kv.Key != that1.Key {
		if kv.Key < that1.Key {
			return -1
		}
		return 1
	}
	if kv.ValueType != that1.ValueType {
		if kv.ValueType < that1.ValueType {
			return -1
		}
		return 1
	}
	if kv.ValueString != that1.ValueString {
		if kv.ValueString < that1.ValueString {
			return -1
		}
		return 1
	}
	if kv.ValueBool != that1.ValueBool {
		if !kv.ValueBool {
			return -1
		}
		return 1
	}
	if kv.ValueInt64 != that1.ValueInt64 {
		if kv.ValueInt64 < that1.ValueInt64 {
			return -1
		}
		return 1
	}
	if kv.ValueFloat64 != that1.ValueFloat64 {
		if kv.ValueFloat64 < that1.ValueFloat64 {
			return -1
		}
		return 1
	}
	if c := bytes.Compare(kv.ValueBinary, that1.ValueBinary); c != 0 {
		return c
	}
	return 0
}

func (kv *KeyValue) Equal(that any) bool {
	if that == nil {
		return kv == nil
	}
	that1, ok := that.(*KeyValue)
	if !ok {
		that2, ok := that.(KeyValue)
		if !ok {
			return false
		}
		that1 = &that2
	}
	if that1 == nil {
		return kv == nil
	} else if kv == nil {
		return false
	}
	if kv.Key != that1.Key {
		return false
	}
	if kv.ValueType != that1.ValueType {
		return false
	}
	if kv.ValueString != that1.ValueString {
		return false
	}
	if kv.ValueBool != that1.ValueBool {
		return false
	}
	if kv.ValueInt64 != that1.ValueInt64 {
		return false
	}
	if kv.ValueFloat64 != that1.ValueFloat64 {
		return false
	}
	if !bytes.Equal(kv.ValueBinary, that1.ValueBinary) {
		return false
	}
	return true
}

func (kv *KeyValue) AsString() string {
	switch kv.ValueType {
	case stringType:
		return kv.ValueString
	case boolType:
		if kv.ValueBool {
			return "true"
		}
		return "false"
	case int64Type:
		return strconv.FormatInt(kv.ValueInt64, 10)
	case float64Type:
		return strconv.FormatFloat(kv.ValueFloat64, 'g', 10, 64)
	case binaryType:
		return hex.EncodeToString(kv.ValueBinary)
	default:
		return "unknown type " + kv.ValueType
	}
}

func SortKVs(kvs []KeyValue) {
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Compare(kvs[j]) < 0
	})
}

// Log is the UDT representation of a Jaeger Log.
type Log struct {
	Timestamp int64      `cql:"ts"` // microseconds since epoch
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
func (t TraceID) ToDomain() model.TraceID {
	traceIDHigh := binary.BigEndian.Uint64(t[:8])
	traceIDLow := binary.BigEndian.Uint64(t[8:])
	return model.NewTraceID(traceIDHigh, traceIDLow)
}

// String returns hex string representation of the trace ID.
func (t TraceID) String() string {
	return t.ToDomain().String()
}
