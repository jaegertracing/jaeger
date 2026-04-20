// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestSpanHash(t *testing.T) {
	baseSpan := Span{
		TraceID:       TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:        123,
		OperationName: "test",
		StartTime:     456,
		Duration:      789,
		Tags: []KeyValue{
			{Key: "b", ValueType: StringType, ValueString: "v2"},
			{Key: "a", ValueType: StringType, ValueString: "v1"},
		},
		Process: Process{
			ServiceName: "svc",
			Tags: []KeyValue{
				{Key: "d", ValueType: StringType, ValueString: "v4"},
				{Key: "c", ValueType: StringType, ValueString: "v3"},
			},
		},
		Logs: []Log{
			{Timestamp: 2, Fields: []KeyValue{{Key: "f", ValueType: StringType, ValueString: "v6"}}},
			{Timestamp: 1, Fields: []KeyValue{{Key: "e", ValueType: StringType, ValueString: "v5"}}},
		},
		Refs: []SpanRef{
			{RefType: ChildOf, SpanID: 2},
			{RefType: ChildOf, SpanID: 1},
		},
		SpanHash: 1000,
	}

	getHash := func(s Span) []byte {
		var buf bytes.Buffer
		err := s.Hash(&buf)
		require.NoError(t, err)
		return buf.Bytes()
	}

	baseHash := getHash(baseSpan)

	tests := []struct {
		name     string
		mutate   func(s *Span)
		shouldEq bool
	}{
		{
			name:     "determinism",
			mutate:   func(_ *Span) {},
			shouldEq: true,
		},
		{
			name: "ignore existing SpanHash",
			mutate: func(s *Span) {
				s.SpanHash = 2000
			},
			shouldEq: true,
		},
		{
			name: "stable under tag reordering",
			mutate: func(s *Span) {
				s.Tags[0], s.Tags[1] = s.Tags[1], s.Tags[0]
			},
			shouldEq: true,
		},
		{
			name: "stable under process tag reordering",
			mutate: func(s *Span) {
				s.Process.Tags[0], s.Process.Tags[1] = s.Process.Tags[1], s.Process.Tags[0]
			},
			shouldEq: true,
		},
		{
			name: "stable under log reordering",
			mutate: func(s *Span) {
				s.Logs[0], s.Logs[1] = s.Logs[1], s.Logs[0]
			},
			shouldEq: true,
		},
		{
			name: "stable under ref reordering",
			mutate: func(s *Span) {
				s.Refs[0], s.Refs[1] = s.Refs[1], s.Refs[0]
			},
			shouldEq: true,
		},
		{
			name: "different TraceID",
			mutate: func(s *Span) {
				s.TraceID[0] = 255
			},
			shouldEq: false,
		},
		{
			name: "different SpanID",
			mutate: func(s *Span) {
				s.SpanID = 999
			},
			shouldEq: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testSpan := baseSpan
			// Deep copy slices to avoid side effects
			testSpan.Tags = append([]KeyValue(nil), baseSpan.Tags...)
			testSpan.Process.Tags = append([]KeyValue(nil), baseSpan.Process.Tags...)
			testSpan.Logs = append([]Log(nil), baseSpan.Logs...)
			testSpan.Refs = append([]SpanRef(nil), baseSpan.Refs...)

			tc.mutate(&testSpan)
			currentHash := getHash(testSpan)
			if tc.shouldEq {
				assert.Equal(t, baseHash, currentHash)
			} else {
				assert.NotEqual(t, baseHash, currentHash)
			}
		})
	}
}

func TestCompareKVs(t *testing.T) {
	tests := []struct {
		name   string
		a, b   []KeyValue
		expect int
	}{
		{
			name:   "equal empty",
			a:      []KeyValue{},
			b:      []KeyValue{},
			expect: 0,
		},
		{
			name:   "length mismatch",
			a:      []KeyValue{{Key: "a"}},
			b:      []KeyValue{},
			expect: 1,
		},
		{
			name:   "content mismatch",
			a:      []KeyValue{{Key: "a"}},
			b:      []KeyValue{{Key: "b"}},
			expect: -1,
		},
		{
			name:   "equal content",
			a:      []KeyValue{{Key: "a", ValueType: StringType, ValueString: "v"}},
			b:      []KeyValue{{Key: "a", ValueType: StringType, ValueString: "v"}},
			expect: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, compareKVs(tc.a, tc.b))
		})
	}
}

func TestSortLogs(t *testing.T) {
	logs := []Log{
		{Timestamp: 2, Fields: []KeyValue{{Key: "a"}}},
		{Timestamp: 1, Fields: []KeyValue{{Key: "b"}}},
		{Timestamp: 2, Fields: []KeyValue{{Key: "b"}}},
	}
	sortLogs(logs)
	assert.Equal(t, int64(1), logs[0].Timestamp)
	assert.Equal(t, int64(2), logs[1].Timestamp)
	assert.Equal(t, "a", logs[1].Fields[0].Key)
	assert.Equal(t, int64(2), logs[2].Timestamp)
	assert.Equal(t, "b", logs[2].Fields[0].Key)
}

func TestSortSpanRefs(t *testing.T) {
	refs := []SpanRef{
		{TraceID: TraceID{2}, SpanID: 1, RefType: ChildOf},
		{TraceID: TraceID{1}, SpanID: 2, RefType: ChildOf},
		{TraceID: TraceID{1}, SpanID: 1, RefType: FollowsFrom},
		{TraceID: TraceID{1}, SpanID: 1, RefType: ChildOf},
	}
	sortSpanRefs(refs)
	assert.Equal(t, byte(1), refs[0].TraceID[0])
	assert.Equal(t, int64(1), refs[0].SpanID)
	assert.Equal(t, ChildOf, refs[0].RefType)

	assert.Equal(t, byte(1), refs[1].TraceID[0])
	assert.Equal(t, int64(1), refs[1].SpanID)
	assert.Equal(t, FollowsFrom, refs[1].RefType)

	assert.Equal(t, byte(1), refs[2].TraceID[0])
	assert.Equal(t, int64(2), refs[2].SpanID)

	assert.Equal(t, byte(2), refs[3].TraceID[0])
}
