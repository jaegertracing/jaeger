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

package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"encoding/json"

	"github.com/uber/jaeger/model"
)

func TestValueTypeToFromString(t *testing.T) {
	badValueType := model.ValueType(-1)
	testCases := []struct {
		v model.ValueType
		s string
	}{
		{model.StringType, "string"},
		{model.BoolType, "bool"},
		{model.Int64Type, "int64"},
		{model.Float64Type, "float64"},
		{model.BinaryType, "binary"},
		{badValueType, "<invalid>"},
	}
	for _, testCase := range testCases {
		assert.Equal(t, testCase.s, testCase.v.String(), testCase.s)
		v2, err := model.ValueTypeFromString(testCase.s)
		if testCase.v == badValueType {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err, testCase.s)
			assert.Equal(t, testCase.v, v2, testCase.s)
		}
	}
}

func TestValueTypeToFromJSON(t *testing.T) {
	kv := model.Int64("x", 123)
	out, err := json.Marshal(kv)
	assert.NoError(t, err)
	assert.Equal(t, `{"key":"x","vType":"int64","vNum":123}`, string(out))
	var kv2, kv3 model.KeyValue
	if assert.NoError(t, json.Unmarshal(out, &kv2)) {
		assert.True(t, kv.Equal(&kv2))
		assert.Equal(t, kv, kv2)
	}
	err = json.Unmarshal([]byte(`{"key":"x","vType":"BAD","vNum":123}`), &kv3)
	assert.EqualError(t, err, "not a valid ValueType string BAD")
}

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

func TestKeyValueAsStringAndValue(t *testing.T) {
	longString := `Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues `
	expectedBinaryStr := `42656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e64...`
	testCases := []struct {
		kv  model.KeyValue
		str string
		val interface{}
	}{
		{kv: model.String("x", "Bender is great!"), str: "Bender is great!", val: "Bender is great!"},
		{kv: model.Bool("x", false), str: "false", val: false},
		{kv: model.Bool("x", true), str: "true", val: true},
		{kv: model.Int64("x", 3000), str: "3000", val: int64(3000)},
		{kv: model.Int64("x", -1947), str: "-1947", val: int64(-1947)},
		{kv: model.Float64("x", 3.14159265359), str: "3.141592654", val: float64(3.14159265359)},
		{kv: model.Binary("x", []byte("Bender")), str: "42656e646572", val: []byte("Bender")},
		{kv: model.Binary("x", []byte(longString)), str: expectedBinaryStr, val: []byte(longString)},
	}
	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.str, func(t *testing.T) {
			assert.Equal(t, testCase.str, testCase.kv.AsString())
			assert.Equal(t, testCase.val, testCase.kv.Value())
		})
	}
	t.Run("invalid type", func(t *testing.T) {
		kv := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		assert.Equal(t, "unknown type -1", kv.AsString())
		assert.EqualError(t, kv.Value().(error), "unknown type -1")
	})
}
