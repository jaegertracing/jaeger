// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestTagInsertionString(t *testing.T) {
	v := TagInsertion{"x", "y", "z"}
	assert.Equal(t, "x:y:z", v.String())
}

func TestTraceIDString(t *testing.T) {
	id := TraceIDFromDomain(model.NewTraceID(1, 1))
	assert.Equal(t, "00000000000000010000000000000001", id.String())
}

func TestKeyValueCompare(t *testing.T) {
	tests := []struct {
		name   string
		kv1    *KeyValue
		kv2    any
		result int
	}{
		{
			name:   "BothNil",
			kv1:    nil,
			kv2:    nil,
			result: 0,
		},
		{
			name:   "Nil_vs_Value",
			kv1:    nil,
			kv2:    &KeyValue{Key: "k", ValueType: "string"},
			result: -1,
		},
		{
			name:   "Pointer_vs_Value",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    KeyValue{Key: "m", ValueType: "string"},
			result: -1,
		},
		{
			name:   "Value_vs_Nil",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    nil,
			result: 1,
		},
		{
			name:   "TypedNil_vs_Value",
			kv1:    (*KeyValue)(nil),
			kv2:    &KeyValue{Key: "k", ValueType: "string"},
			result: -1,
		},
		{
			name:   "TypedNil_vs_TypedNil",
			kv1:    (*KeyValue)(nil),
			kv2:    (*KeyValue)(nil),
			result: 0,
		},
		{
			name:   "Value_vs_TypedNil",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    (*KeyValue)(nil),
			result: 1,
		},
		{
			name:   "InvalidType",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    123,
			result: 1,
		},
		{
			name: "Equal",
			kv1: &KeyValue{
				Key:         "k",
				ValueType:   "string",
				ValueString: "hello",
			},
			kv2: &KeyValue{
				Key:         "k",
				ValueType:   "string",
				ValueString: "hello",
			},
			result: 0,
		},
		{
			name:   "KeyMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    &KeyValue{Key: "a", ValueType: "string"},
			result: 1,
		},
		{
			name:   "ValueTypeMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "z"},
			kv2:    &KeyValue{Key: "k", ValueType: "a"},
			result: 1,
		},
		{
			name:   "ValueStringMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "string", ValueString: "zzz"},
			kv2:    &KeyValue{Key: "k", ValueType: "string", ValueString: "aaa"},
			result: 1,
		},
		{
			name:   "ValueBoolMismatch_After",
			kv1:    &KeyValue{Key: "k", ValueType: "bool", ValueBool: true},
			kv2:    &KeyValue{Key: "k", ValueType: "bool", ValueBool: false},
			result: 1,
		},
		{
			name:   "ValueBoolMismatch_Before",
			kv1:    &KeyValue{Key: "k", ValueType: "bool", ValueBool: false},
			kv2:    &KeyValue{Key: "k", ValueType: "bool", ValueBool: true},
			result: -1,
		},
		{
			name:   "ValueInt64Mismatch_After",
			kv1:    &KeyValue{Key: "k", ValueType: "int64", ValueInt64: 10},
			kv2:    &KeyValue{Key: "k", ValueType: "int64", ValueInt64: 5},
			result: 5,
		},
		{
			name:   "ValueFloat64Mismatch_After",
			kv1:    &KeyValue{Key: "k", ValueType: "float64", ValueFloat64: 1.5},
			kv2:    &KeyValue{Key: "k", ValueType: "float64", ValueFloat64: 0.5},
			result: 1,
		},
		{
			name:   "ValueFloat64Mismatch_Before",
			kv1:    &KeyValue{Key: "k", ValueType: "float64", ValueFloat64: 0.5},
			kv2:    &KeyValue{Key: "k", ValueType: "float64", ValueFloat64: 1.5},
			result: -1,
		},
		{
			name:   "ValueBinaryMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
			kv2:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 4}},
			result: bytes.Compare([]byte{1, 2, 3}, []byte{1, 2, 4}),
		},
		{
			name:   "ValueBinaryEqual",
			kv1:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
			kv2:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
			result: 0,
		},
		{
			name:   "UnknownType",
			kv1:    &KeyValue{Key: "k", ValueType: "random", ValueString: "hello"},
			kv2:    &KeyValue{Key: "k", ValueType: "random", ValueString: "hellobig"},
			result: -1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.result, tc.kv1.Compare(tc.kv2))
		})
	}
}

func TestKeyValueEqual(t *testing.T) {
	tests := []struct {
		name   string
		kv1    *KeyValue
		kv2    any
		result bool
	}{
		{
			name:   "BothNil",
			kv1:    nil,
			kv2:    nil,
			result: true,
		},
		{
			name:   "Nil_vs_Value",
			kv1:    nil,
			kv2:    &KeyValue{Key: "k", ValueType: "string"},
			result: false,
		},
		{
			name:   "Value_vs_Nil",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    nil,
			result: false,
		},
		{
			name:   "TypedNil_vs_Value",
			kv1:    (*KeyValue)(nil),
			kv2:    &KeyValue{Key: "k", ValueType: "string"},
			result: false,
		},
		{
			name:   "Value_vs_TypedNil",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    (*KeyValue)(nil),
			result: false,
		},
		{
			name:   "InvalidType",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    123,
			result: false,
		},
		{
			name: "Equal",
			kv1: &KeyValue{
				Key:         "k",
				ValueType:   "string",
				ValueString: "hello",
			},
			kv2: &KeyValue{
				Key:         "k",
				ValueType:   "string",
				ValueString: "hello",
			},
			result: true,
		},
		{
			name:   "KeyMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "string"},
			kv2:    &KeyValue{Key: "a", ValueType: "string"},
			result: false,
		},
		{
			name:   "ValueTypeMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "z"},
			kv2:    &KeyValue{Key: "k", ValueType: "a"},
			result: false,
		},
		{
			name:   "ValueStringMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "string", ValueString: "zzz"},
			kv2:    &KeyValue{Key: "k", ValueType: "string", ValueString: "aaa"},
			result: false,
		},
		{
			name:   "ValueBoolMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "bool", ValueBool: true},
			kv2:    &KeyValue{Key: "k", ValueType: "bool", ValueBool: false},
			result: false,
		},
		{
			name:   "ValueInt64Mismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "int64", ValueInt64: 10},
			kv2:    &KeyValue{Key: "k", ValueType: "int64", ValueInt64: 5},
			result: false,
		},
		{
			name:   "ValueFloat64Mismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "float64", ValueFloat64: 1.5},
			kv2:    &KeyValue{Key: "k", ValueType: "float64", ValueFloat64: 0.5},
			result: false,
		},
		{
			name:   "ValueBinaryMismatch",
			kv1:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
			kv2:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 4}},
			result: false,
		},
		{
			name:   "ValueBinaryEqual",
			kv1:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
			kv2:    &KeyValue{Key: "k", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
			result: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.result, tc.kv1.Equal(tc.kv2))
		})
	}
}

func TestKeyValueAsString(t *testing.T) {
	tests := []struct {
		name   string
		kv     KeyValue
		expect string
	}{
		{
			name: "StringType",
			kv: KeyValue{
				Key:         "k",
				ValueType:   StringType,
				ValueString: "hello",
			},
			expect: "hello",
		},
		{
			name: "BoolTrue",
			kv: KeyValue{
				Key:       "k",
				ValueType: BoolType,
				ValueBool: true,
			},
			expect: "true",
		},
		{
			name: "BoolFalse",
			kv: KeyValue{
				Key:       "k",
				ValueType: BoolType,
				ValueBool: false,
			},
			expect: "false",
		},
		{
			name: "Int64Type",
			kv: KeyValue{
				Key:        "k",
				ValueType:  Int64Type,
				ValueInt64: 12345,
			},
			expect: "12345",
		},
		{
			name: "Float64Type",
			kv: KeyValue{
				Key:          "k",
				ValueType:    Float64Type,
				ValueFloat64: 12.34,
			},
			expect: "12.34",
		},
		{
			name: "BinaryType",
			kv: KeyValue{
				Key:         "k",
				ValueType:   BinaryType,
				ValueBinary: []byte{0xAB, 0xCD, 0xEF},
			},
			expect: "abcdef",
		},
		{
			name: "UnknownType",
			kv: KeyValue{
				Key:       "k",
				ValueType: "random-type",
			},
			expect: "unknown type random-type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, tc.kv.AsString())
		})
	}
}

func TestSortKVs_WithKey(t *testing.T) {
	kvs := []KeyValue{
		{Key: "z", ValueType: "string", ValueString: "hello"},
		{Key: "y", ValueType: "bool", ValueBool: true},
		{Key: "x", ValueType: "int64", ValueInt64: 99},
		{Key: "w", ValueType: "double", ValueFloat64: 1.23},
		{Key: "v", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
		{Key: "m", ValueType: "string", ValueString: "abc"},
	}
	SortKVs(kvs)
	want := []string{"m", "v", "w", "x", "y", "z"}
	for i, kv := range kvs {
		assert.Equal(t, want[i], kv.Key)
	}
}

func TestSortKVs_WithType(t *testing.T) {
	kvs := []KeyValue{
		{Key: "m", ValueType: "string", ValueString: "hello"},
		{Key: "m", ValueType: "bool", ValueBool: true},
		{Key: "m", ValueType: "int64", ValueInt64: 99},
		{Key: "m", ValueType: "double", ValueFloat64: 1.23},
		{Key: "m", ValueType: "binary", ValueBinary: []byte{1, 2, 3}},
	}
	SortKVs(kvs)
	want := []string{"binary", "bool", "double", "int64", "string"}
	for i, kv := range kvs {
		assert.Equal(t, want[i], kv.ValueType)
	}
}

func TestSortKVs_WithValue(t *testing.T) {
	kvs := []KeyValue{
		{Key: "m", ValueType: "string", ValueString: "a"},
		{Key: "m", ValueType: "string", ValueString: "b"},
		{Key: "m", ValueType: "string", ValueString: "c"},
		{Key: "m", ValueType: "string", ValueString: "d"},
		{Key: "m", ValueType: "string", ValueString: "e"},
	}
	SortKVs(kvs)
	want := []string{"a", "b", "c", "d", "e"}
	for i, kv := range kvs {
		assert.Equal(t, want[i], kv.ValueString)
	}
}
