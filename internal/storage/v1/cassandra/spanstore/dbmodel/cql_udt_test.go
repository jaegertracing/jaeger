// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/pkg/cassandra/gocql/testutils"
)

func TestDBModelUDTMarshall(t *testing.T) {
	floatVal := float64(123.456)
	floatBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(floatBytes, uint64(math.Float64bits(floatVal)))

	kv := &KeyValue{
		Key:          "key",
		ValueType:    "value_type",
		ValueString:  "string_value",
		ValueBool:    true,
		ValueInt64:   123,
		ValueFloat64: floatVal,
		ValueBinary:  []byte("value_binary"),
	}

	log := &Log{
		Timestamp: 123,
	}

	spanRef := &SpanRef{
		RefType: "childOf",
		TraceID: TraceIDFromDomain(model.NewTraceID(0, 1)),
		SpanID:  123,
	}

	proc := &Process{
		ServiceName: "google",
	}

	testCases := []testutils.UDTTestCase{
		{
			Obj:     kv,
			New:     func() gocql.UDTUnmarshaler { return &KeyValue{} },
			ObjName: "KeyValue",
			Fields: []testutils.UDTField{
				{Name: "key", Type: gocql.TypeAscii, ValIn: []byte("key"), Err: false},
				{Name: "value_type", Type: gocql.TypeAscii, ValIn: []byte("value_type"), Err: false},
				{Name: "value_string", Type: gocql.TypeAscii, ValIn: []byte("string_value"), Err: false},
				{Name: "value_bool", Type: gocql.TypeBoolean, ValIn: []byte{1}, Err: false},
				{Name: "value_long", Type: gocql.TypeBigInt, ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 123}, Err: false},
				{Name: "value_double", Type: gocql.TypeDouble, ValIn: floatBytes, Err: false},
				{Name: "value_binary", Type: gocql.TypeBlob, ValIn: []byte("value_binary"), Err: false},
				{Name: "wrong-field", Err: true},
			},
		},
		{
			Obj:     log,
			New:     func() gocql.UDTUnmarshaler { return &Log{} },
			ObjName: "Log",
			Fields: []testutils.UDTField{
				{Name: "ts", Type: gocql.TypeBigInt, ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 123}, Err: false},
				{Name: "fields", Type: gocql.TypeAscii, Err: true}, // for coverage only
				{Name: "wrong-field", Err: true},
			},
		},
		{
			Obj:     spanRef,
			New:     func() gocql.UDTUnmarshaler { return &SpanRef{} },
			ObjName: "SpanRef",
			Fields: []testutils.UDTField{
				{Name: "ref_type", Type: gocql.TypeAscii, ValIn: []byte("childOf"), Err: false},
				{Name: "trace_id", Type: gocql.TypeBlob, ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Err: false},
				{Name: "span_id", Type: gocql.TypeBigInt, ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 123}, Err: false},
				{Name: "wrong-field", Err: true},
			},
		},
		{
			Obj:     proc,
			New:     func() gocql.UDTUnmarshaler { return &Process{} },
			ObjName: "Process",
			Fields: []testutils.UDTField{
				{Name: "service_name", Type: gocql.TypeAscii, ValIn: []byte("google"), Err: false},
				{Name: "tags", Type: gocql.TypeAscii, Err: true}, // for coverage only
				{Name: "wrong-field", Err: true},
			},
		},
	}
	for _, testCase := range testCases {
		testCase.Run(t)
	}

	dbid := TraceID{}
	assert.Equal(t, ErrTraceIDWrongLength, dbid.UnmarshalCQL(nil, nil))
}
