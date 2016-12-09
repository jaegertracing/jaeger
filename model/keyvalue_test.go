package model_test

import (
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

func TestKeyValueIsLess(t *testing.T) {
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
				assert.False(t, model.IsLess(&testCase.kv1, &testCase.kv2))
			} else {
				assert.True(t, model.IsLess(&testCase.kv1, &testCase.kv2))
			}
			assert.False(t, model.IsLess(&testCase.kv2, &testCase.kv1))
		})
	}
	t.Run("invalid type", func(t *testing.T) {
		v1 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		v2 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
		assert.False(t, model.IsLess(&v1, &v2))
		assert.False(t, model.IsLess(&v2, &v1))
	})
}
