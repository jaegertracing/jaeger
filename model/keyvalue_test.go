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

package model_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestKeyValueString(t *testing.T) {
	kv := model.String("x", "y")
	assert.Equal(t, "x", kv.Key)
	assert.Equal(t, model.StringType, kv.VType)
	assert.Equal(t, "y", kv.VStr)
	assert.False(t, kv.Bool())
	assert.Equal(t, int64(0), kv.Int64())
	assert.Equal(t, float64(0), kv.Float64())
	assert.Nil(t, kv.Binary())
}

func TestKeyValueBool(t *testing.T) {
	kv := model.Bool("x", true)
	assert.Equal(t, "x", kv.Key)
	assert.Equal(t, model.BoolType, kv.VType)
	assert.True(t, kv.Bool())
}

func TestKeyValueInt64(t *testing.T) {
	kv := model.Int64("x", 123)
	assert.Equal(t, "x", kv.Key)
	assert.Equal(t, model.Int64Type, kv.VType)
	assert.Equal(t, int64(123), kv.Int64())
}

func TestKeyValueFloat64(t *testing.T) {
	kv := model.Float64("x", 123.345)
	assert.Equal(t, "x", kv.Key)
	assert.Equal(t, model.Float64Type, kv.VType)
	assert.Equal(t, 123.345, kv.Float64())
}

func TestKeyValueBinary(t *testing.T) {
	kv := model.Binary("x", []byte{123, 45})
	assert.Equal(t, "x", kv.Key)
	assert.Equal(t, model.BinaryType, kv.VType)
	assert.Equal(t, []byte{123, 45}, kv.Binary())
}

func TestKeyValueIsLessAndEqual(t *testing.T) {
	testCases := []struct {
		name  string
		kv1   model.KeyValue
		kv2   model.KeyValue
		equal bool
	}{
		{name: "different keys", kv1: model.String("x", "123"), kv2: model.String("y", "123")},
		{name: "same key different types", kv1: model.String("x", "123"), kv2: model.Int64("x", 123)},
		{name: "different string values", kv1: model.String("x", "123"), kv2: model.String("x", "567")},
		{name: "different bool values", kv1: model.Bool("x", false), kv2: model.Bool("x", true)},
		{name: "different int64 values", kv1: model.Int64("x", 123), kv2: model.Int64("x", 567)},
		{name: "different float64 values", kv1: model.Float64("x", 123), kv2: model.Float64("x", 567)},
		{name: "different blob length", kv1: model.Binary("x", []byte{1, 2}), kv2: model.Binary("x", []byte{1, 2, 3})},
		{name: "different blob values", kv1: model.Binary("x", []byte{1, 2, 3}), kv2: model.Binary("x", []byte{1, 2, 4})},
		{name: "empty blob", kv1: model.Binary("x", nil), kv2: model.Binary("x", nil), equal: true},
		{name: "identical blob", kv1: model.Binary("x", []byte{1, 2, 3}), kv2: model.Binary("x", []byte{1, 2, 3}), equal: true},
	}
	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.equal {
				assert.False(t, testCase.kv1.IsLess(&testCase.kv2))
				assert.True(t, testCase.kv1.Equal(&testCase.kv2))
				assert.True(t, testCase.kv2.Equal(&testCase.kv1))
			} else {
				assert.True(t, testCase.kv1.IsLess(&testCase.kv2))
				assert.False(t, testCase.kv1.Equal(&testCase.kv2))
				assert.False(t, testCase.kv2.Equal(&testCase.kv1))
			}
			assert.False(t, testCase.kv2.IsLess(&testCase.kv1))
		})
	}
	t.Run("invalid type", func(t *testing.T) {
		v1 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		v2 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		assert.False(t, v1.IsLess(&v2))
		assert.False(t, v2.IsLess(&v1))
		assert.False(t, v1.Equal(&v2))
		assert.False(t, v2.Equal(&v1))
	})
}

// No memory allocations for IsLess and Equal
// 18.6 ns/op	       0 B/op	       0 allocs/op
func BenchmarkKeyValueIsLessAndEquals(b *testing.B) {
	v1 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	v2 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	for i := 0; i < b.N; i++ {
		v1.IsLess(&v2)
		v2.IsLess(&v1)
		v1.Equal(&v2)
		v2.Equal(&v1)
	}
}

// No memory allocations for Sort (1 alloc comes from the algorithm, not from comparisons)
// 107 ns/op	      32 B/op	       1 allocs/op
func BenchmarkKeyValuesSort(b *testing.B) {
	v1 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	v2 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	list := model.KeyValues(make([]model.KeyValue, 5))
	for i := 0; i < b.N; i++ {
		list[0], list[1], list[2], list[3], list[4] = v2, v1, v2, v1, v2
		list.Sort()
	}
}

func TestKeyValueAsString(t *testing.T) {
	testCases := []struct {
		kv  model.KeyValue
		str string
	}{
		{kv: model.String("x", "Bender is great!"), str: "Bender is great!"},
		{kv: model.Bool("x", false), str: "false"},
		{kv: model.Bool("x", true), str: "true"},
		{kv: model.Int64("x", 3000), str: "3000"},
		{kv: model.Int64("x", -1947), str: "-1947"},
		{kv: model.Float64("x", 3.14159265359), str: "3.141592654"},
		{kv: model.Binary("x", []byte("Bender")), str: "42656e646572"},
		{kv: model.Binary("x", []byte("Bender Bending Rodrigues")), str: "42656e6465722042656e64696e672052..."},
	}
	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.str, func(t *testing.T) {
			assert.Equal(t, testCase.str, testCase.kv.AsString())
		})
	}
	t.Run("invalid type", func(t *testing.T) {
		kv := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		assert.Equal(t, "unknown type -1", kv.AsString())
	})
}

func TestSort(t *testing.T) {
	input := model.KeyValues{
		model.String("x", "z"),
		model.String("x", "y"),
		model.Int64("a", 2),
		model.Int64("a", 1),
		model.Float64("x", 2),
		model.Float64("x", 1),
		model.Bool("x", true),
		model.Bool("x", false),
	}
	expected := model.KeyValues{
		model.Int64("a", 1),
		model.Int64("a", 2),
		model.String("x", "y"),
		model.String("x", "z"),
		model.Bool("x", false),
		model.Bool("x", true),
		model.Float64("x", 1),
		model.Float64("x", 2),
	}
	input.Sort()
	assert.Equal(t, expected, input)
}

func TestFindByKey(t *testing.T) {
	input := model.KeyValues{
		model.String("x", "z"),
		model.String("x", "y"),
		model.Int64("a", 2),
	}

	testCases := []struct {
		key   string
		found bool
		kv    model.KeyValue
	}{
		{"b", false, model.KeyValue{}},
		{"a", true, model.Int64("a", 2)},
		{"x", true, model.String("x", "z")},
	}

	for _, test := range testCases {
		t.Run(fmt.Sprintf("%+v", test), func(t *testing.T) {
			kv, found := input.FindByKey(test.key)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.kv, kv)
		})
	}
}

func TestKeyValuesEqual(t *testing.T) {
	v1 := model.String("s", "abc")
	v2 := model.Int64("i", 123)
	v3 := model.Float64("f", 123.4)
	assert.True(t, model.KeyValues{}.Equal(model.KeyValues{}))
	assert.True(t, model.KeyValues{v1}.Equal(model.KeyValues{v1}))
	assert.True(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v1, v2}))
	assert.False(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v2, v1}))
	assert.False(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v1, v3}))
	assert.False(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v1, v2, v3}))
}
