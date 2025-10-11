// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestExtractAttributes(t *testing.T) {
	tests := []struct {
		name     string
		attrs    pcommon.Map
		expected AttributesGroup
	}{
		{
			name: "bool attributes",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutBool("key1", true)
				m.PutBool("key2", false)
				return m
			}(),
			expected: AttributesGroup{
				BoolKeys:   []string{"key1", "key2"},
				BoolValues: []bool{true, false},
			},
		},
		{
			name: "double attributes",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutDouble("key1", 1.1)
				m.PutDouble("key2", 2.2)
				return m
			}(),
			expected: AttributesGroup{
				DoubleKeys:   []string{"key1", "key2"},
				DoubleValues: []float64{1.1, 2.2},
			},
		},
		{
			name: "int attributes",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutInt("key1", 1)
				m.PutInt("key2", 2)
				return m
			}(),
			expected: AttributesGroup{
				IntKeys:   []string{"key1", "key2"},
				IntValues: []int64{1, 2},
			},
		},
		{
			name: "string attributes",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "val1")
				m.PutStr("key2", "val2")
				return m
			}(),
			expected: AttributesGroup{
				StrKeys:   []string{"key1", "key2"},
				StrValues: []string{"val1", "val2"},
			},
		},
		{
			name: "bytes attributes",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutEmptyBytes("key1").FromRaw([]byte{0x01, 0x02})
				m.PutEmptyBytes("key2").FromRaw([]byte{0x03, 0x04})
				return m
			}(),
			expected: AttributesGroup{
				BytesKeys:   []string{"key1", "key2"},
				BytesValues: []string{base64.StdEncoding.EncodeToString([]byte{0x01, 0x02}), base64.StdEncoding.EncodeToString([]byte{0x03, 0x04})},
			},
		},
		{
			name: "mixed attributes",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutBool("bool_key", true)
				m.PutDouble("double_key", 3.14)
				m.PutInt("int_key", 42)
				m.PutStr("str_key", "hello")
				return m
			}(),
			expected: AttributesGroup{
				BoolKeys:     []string{"bool_key"},
				BoolValues:   []bool{true},
				DoubleKeys:   []string{"double_key"},
				DoubleValues: []float64{3.14},
				IntKeys:      []string{"int_key"},
				IntValues:    []int64{42},
				StrKeys:      []string{"str_key"},
				StrValues:    []string{"hello"},
			},
		},
		{
			name: "slice attribute (converted to JSON string)",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				sliceVal := m.PutEmptySlice("slice_key")
				sliceVal.AppendEmpty().SetStr("item1")
				sliceVal.AppendEmpty().SetInt(123)
				return m
			}(),
			expected: AttributesGroup{
				StrKeys:   []string{"slice_key"},
				StrValues: []string{`["item1",123]`},
			},
		},
		{
			name: "map attribute (converted to JSON string)",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				mapVal := m.PutEmptyMap("map_key")
				mapVal.PutStr("nested_str", "val")
				mapVal.PutBool("nested_bool", true)
				return m
			}(),
			expected: AttributesGroup{
				StrKeys: []string{"map_key"},
				// Note: JSON marshaling may produce different key orders
				StrValues: []string{`{"nested_bool":true,"nested_str":"val"}`},
			},
		},
		{
			name:  "empty attributes",
			attrs: pcommon.NewMap(),
			expected: AttributesGroup{
				BoolKeys:     []string{},
				BoolValues:   []bool{},
				DoubleKeys:   []string{},
				DoubleValues: []float64{},
				IntKeys:      []string{},
				IntValues:    []int64{},
				StrKeys:      []string{},
				StrValues:    []string{},
				BytesKeys:    []string{},
				BytesValues:  []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ExtractAttributes(tt.attrs)
			assert.ElementsMatch(t, tt.expected.BoolKeys, actual.BoolKeys)
			assert.ElementsMatch(t, tt.expected.BoolValues, actual.BoolValues)
			assert.ElementsMatch(t, tt.expected.DoubleKeys, actual.DoubleKeys)
			assert.ElementsMatch(t, tt.expected.DoubleValues, actual.DoubleValues)
			assert.ElementsMatch(t, tt.expected.IntKeys, actual.IntKeys)
			assert.ElementsMatch(t, tt.expected.IntValues, actual.IntValues)
			assert.ElementsMatch(t, tt.expected.StrKeys, actual.StrKeys)

			// For string values, handle JSON with different key ordering
			if tt.name == "map attribute (converted to JSON string)" {
				// Parse and compare as maps
				assert.Len(t, actual.StrValues, 1)
				var actualMap, expectedMap map[string]interface{}
				assert.NoError(t, json.Unmarshal([]byte(actual.StrValues[0]), &actualMap))
				assert.NoError(t, json.Unmarshal([]byte(tt.expected.StrValues[0]), &expectedMap))
				assert.Equal(t, expectedMap, actualMap)
			} else {
				assert.ElementsMatch(t, tt.expected.StrValues, actual.StrValues)
			}

			assert.ElementsMatch(t, tt.expected.BytesKeys, actual.BytesKeys)
			assert.ElementsMatch(t, tt.expected.BytesValues, actual.BytesValues)
		})
	}
}

func TestCombineAttributes(t *testing.T) {
	resource := AttributesGroup{
		BoolKeys:   []string{"res_bool"},
		BoolValues: []bool{true},
		StrKeys:    []string{"res_str"},
		StrValues:  []string{"resource"},
	}

	scope := AttributesGroup{
		DoubleKeys:   []string{"scope_double"},
		DoubleValues: []float64{1.5},
	}

	span := AttributesGroup{
		IntKeys:   []string{"span_int"},
		IntValues: []int64{42},
	}

	result := CombineAttributes(resource, scope, span)

	// Verify all attributes are present
	assert.ElementsMatch(t, []string{"res_bool"}, result.BoolKeys)
	assert.ElementsMatch(t, []bool{true}, result.BoolValues)
	assert.ElementsMatch(t, []string{"scope_double"}, result.DoubleKeys)
	assert.ElementsMatch(t, []float64{1.5}, result.DoubleValues)
	assert.ElementsMatch(t, []string{"span_int"}, result.IntKeys)
	assert.ElementsMatch(t, []int64{42}, result.IntValues)
	assert.ElementsMatch(t, []string{"res_str"}, result.StrKeys)
	assert.ElementsMatch(t, []string{"resource"}, result.StrValues)
}

func TestCombineAttributesEmpty(t *testing.T) {
	empty := AttributesGroup{}
	result := CombineAttributes(empty, empty, empty)

	assert.Empty(t, result.BoolKeys)
	assert.Empty(t, result.BoolValues)
	assert.Empty(t, result.DoubleKeys)
	assert.Empty(t, result.DoubleValues)
	assert.Empty(t, result.IntKeys)
	assert.Empty(t, result.IntValues)
	assert.Empty(t, result.StrKeys)
	assert.Empty(t, result.StrValues)
	assert.Empty(t, result.BytesKeys)
	assert.Empty(t, result.BytesValues)
}

func TestConvertValueToInterface(t *testing.T) {
	tests := []struct {
		name     string
		value    pcommon.Value
		expected interface{}
	}{
		{
			name: "bool",
			value: func() pcommon.Value {
				v := pcommon.NewValueBool(true)
				return v
			}(),
			expected: true,
		},
		{
			name: "double",
			value: func() pcommon.Value {
				v := pcommon.NewValueDouble(3.14)
				return v
			}(),
			expected: 3.14,
		},
		{
			name: "int",
			value: func() pcommon.Value {
				v := pcommon.NewValueInt(42)
				return v
			}(),
			expected: int64(42),
		},
		{
			name: "string",
			value: func() pcommon.Value {
				v := pcommon.NewValueStr("hello")
				return v
			}(),
			expected: "hello",
		},
		{
			name: "bytes",
			value: func() pcommon.Value {
				v := pcommon.NewValueBytes()
				v.Bytes().FromRaw([]byte{0x01, 0x02})
				return v
			}(),
			expected: base64.StdEncoding.EncodeToString([]byte{0x01, 0x02}),
		},
		{
			name: "slice",
			value: func() pcommon.Value {
				v := pcommon.NewValueSlice()
				s := v.Slice()
				s.AppendEmpty().SetStr("item1")
				s.AppendEmpty().SetInt(123)
				return v
			}(),
			expected: []interface{}{"item1", int64(123)},
		},
		{
			name: "map",
			value: func() pcommon.Value {
				v := pcommon.NewValueMap()
				m := v.Map()
				m.PutStr("key1", "val1")
				m.PutInt("key2", 456)
				return v
			}(),
			expected: map[string]interface{}{
				"key1": "val1",
				"key2": int64(456),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := convertValueToInterface(tt.value)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestNestedSliceConversion(t *testing.T) {
	v := pcommon.NewValueSlice()
	s := v.Slice()

	// Add nested slice
	nestedSlice := s.AppendEmpty()
	nestedSlice.SetEmptySlice().AppendEmpty().SetStr("nested")

	// Add nested map
	nestedMap := s.AppendEmpty()
	nestedMap.SetEmptyMap().PutInt("nested_key", 789)

	result := convertValueToInterface(v)
	expected := []interface{}{
		[]interface{}{"nested"},
		map[string]interface{}{"nested_key": int64(789)},
	}

	assert.Equal(t, expected, result)
}

func TestNestedMapConversion(t *testing.T) {
	v := pcommon.NewValueMap()
	m := v.Map()

	// Add nested map
	nestedMap := m.PutEmptyMap("nested_map")
	nestedMap.PutStr("key", "value")

	// Add nested slice
	nestedSlice := m.PutEmptySlice("nested_slice")
	nestedSlice.AppendEmpty().SetInt(1)
	nestedSlice.AppendEmpty().SetInt(2)

	result := convertValueToInterface(v)
	expected := map[string]interface{}{
		"nested_map":   map[string]interface{}{"key": "value"},
		"nested_slice": []interface{}{int64(1), int64(2)},
	}

	assert.Equal(t, expected, result)
}
