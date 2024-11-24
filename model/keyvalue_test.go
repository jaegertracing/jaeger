// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestKeyValueString(t *testing.T) {
	kv := model.String("x", "y")
	assert.Equal(t, "x", kv.Key)
	assert.Equal(t, model.StringType, kv.VType)
	assert.Equal(t, "y", kv.VStr)
	assert.False(t, kv.Bool())
	assert.Equal(t, int64(0), kv.Int64())
	assert.InDelta(t, float64(0), kv.Float64(), 0.01)
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
	assert.InDelta(t, 123.345, kv.Float64(), 1e-4)
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
		v2 := model.KeyValue{Key: "x", VType: model.ValueType(-2)}
		assert.False(t, v1.IsLess(&v2))
		assert.True(t, v2.IsLess(&v1))
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
	expectedBinaryStr := `42656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f6472696775657320`
	expectedBinaryStrLossy := `42656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565732042656e6465722042656e64696e6720526f647269677565730a0942656e6465722042656e64696e6720526f647269677565732042656e64...`
	testCases := []struct {
		name     string
		kv       model.KeyValue
		str      string
		strLossy string
		val      any
	}{
		{name: "Bender is great!", kv: model.String("x", "Bender is great!"), str: "Bender is great!", val: "Bender is great!"},
		{name: "false", kv: model.Bool("x", false), str: "false", val: false},
		{name: "true", kv: model.Bool("x", true), str: "true", val: true},
		{name: "3000", kv: model.Int64("x", 3000), str: "3000", val: int64(3000)},
		{name: "-1947", kv: model.Int64("x", -1947), str: "-1947", val: int64(-1947)},
		{name: "3.141592654", kv: model.Float64("x", 3.14159265359), str: "3.141592654", val: float64(3.14159265359)},
		{name: "42656e646572", kv: model.Binary("x", []byte("Bender")), str: "42656e646572", val: []byte("Bender")},
		{name: "binary string", kv: model.Binary("x", []byte(longString)), str: expectedBinaryStr, val: []byte(longString)},
		{name: "binary string (lossy)", kv: model.Binary("x", []byte(longString)), strLossy: expectedBinaryStrLossy, val: []byte(longString)},
	}
	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.strLossy != "" {
				assert.Equal(t, testCase.strLossy, testCase.kv.AsStringLossy())
			} else {
				assert.Equal(t, testCase.str, testCase.kv.AsString())
			}
			assert.Equal(t, testCase.val, testCase.kv.Value())
		})
	}
	t.Run("invalid type", func(t *testing.T) {
		kv := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		assert.Equal(t, "unknown type -1", kv.AsString())
		require.EqualError(t, kv.Value().(error), "unknown type -1")
	})
}

func TestKeyValueHash(t *testing.T) {
	testCases := []struct {
		kv model.KeyValue
	}{
		{kv: model.String("x", "Bender is great!")},
		{kv: model.Bool("x", true)},
		{kv: model.Int64("x", 3000)},
		{kv: model.Float64("x", 3.14159265359)},
		{kv: model.Binary("x", []byte("Bender"))},
	}
	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.kv.String(), func(t *testing.T) {
			out := new(bytes.Buffer)
			require.NoError(t, testCase.kv.Hash(out))
		})
	}
}
