// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestStringToValueType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected pcommon.ValueType
	}{
		{
			name:     "bool",
			input:    "bool",
			expected: pcommon.ValueTypeBool,
		},
		{
			name:     "BOOL uppercase",
			input:    "BOOL",
			expected: pcommon.ValueTypeBool,
		},
		{
			name:     "double",
			input:    "double",
			expected: pcommon.ValueTypeDouble,
		},
		{
			name:     "int",
			input:    "int",
			expected: pcommon.ValueTypeInt,
		},
		{
			name:     "str",
			input:    "str",
			expected: pcommon.ValueTypeStr,
		},
		{
			name:     "bytes",
			input:    "bytes",
			expected: pcommon.ValueTypeBytes,
		},
		{
			name:     "map",
			input:    "map",
			expected: pcommon.ValueTypeMap,
		},
		{
			name:     "slice",
			input:    "slice",
			expected: pcommon.ValueTypeSlice,
		},
		{
			name:     "unknown string",
			input:    "unknown",
			expected: pcommon.ValueTypeEmpty,
		},
		{
			name:     "empty string",
			input:    "",
			expected: pcommon.ValueTypeEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringToValueType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValueTypeToString(t *testing.T) {
	tests := []struct {
		name     string
		input    pcommon.ValueType
		expected string
	}{
		{
			name:     "ValueTypeBool",
			input:    pcommon.ValueTypeBool,
			expected: "bool",
		},
		{
			name:     "ValueTypeDouble",
			input:    pcommon.ValueTypeDouble,
			expected: "double",
		},
		{
			name:     "ValueTypeInt",
			input:    pcommon.ValueTypeInt,
			expected: "int",
		},
		{
			name:     "ValueTypeStr",
			input:    pcommon.ValueTypeStr,
			expected: "str",
		},
		{
			name:     "ValueTypeBytes",
			input:    pcommon.ValueTypeBytes,
			expected: "bytes",
		},
		{
			name:     "ValueTypeMap",
			input:    pcommon.ValueTypeMap,
			expected: "map",
		},
		{
			name:     "ValueTypeSlice",
			input:    pcommon.ValueTypeSlice,
			expected: "slice",
		},
		{
			name:     "ValueTypeEmpty",
			input:    pcommon.ValueTypeEmpty,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValueTypeToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStringToValueTypeToString(t *testing.T) {
	validTypes := []string{"bool", "double", "int", "str", "bytes", "map", "slice"}
	for _, vt := range validTypes {
		t.Run(vt, func(t *testing.T) {
			valueType := StringToValueType(vt)
			result := ValueTypeToString(valueType)
			assert.Equal(t, vt, result)
		})
	}
}
