// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// TestAttributeTypeOf checks the group type of each value type. Bytes, maps and slices share
// the complex group, and an empty value has no group.
func TestAttributeTypeOf(t *testing.T) {
	for valueType, want := range map[pcommon.ValueType]AttributeType{
		pcommon.ValueTypeBool:   AttributeTypeBool,
		pcommon.ValueTypeDouble: AttributeTypeDouble,
		pcommon.ValueTypeInt:    AttributeTypeInt,
		pcommon.ValueTypeStr:    AttributeTypeStr,
		pcommon.ValueTypeBytes:  AttributeTypeComplex,
		pcommon.ValueTypeMap:    AttributeTypeComplex,
		pcommon.ValueTypeSlice:  AttributeTypeComplex,
		pcommon.ValueTypeEmpty:  "",
	} {
		assert.Equal(t, want, attributeTypeOf(valueType), valueType.String())
	}
}

func TestAttributeType_ValueTypeSQL(t *testing.T) {
	for attributeType, want := range map[AttributeType]string{
		AttributeTypeBool:    "Bool",
		AttributeTypeDouble:  "Float64",
		AttributeTypeInt:     "Int64",
		AttributeTypeStr:     "String",
		AttributeTypeComplex: "String",
	} {
		assert.Equal(t, want, attributeType.valueTypeSQL(), attributeType)
	}
}

// TestAttributeGroup_Column pins the column names of the spans table, by level.
func TestAttributeGroup_Column(t *testing.T) {
	tests := []struct {
		level       expression.Level
		column      string
		arrayColumn string
	}{
		{
			level:  expression.LevelSpan,
			column: "int_attributes",
		},
		{
			level:  expression.LevelResource,
			column: "resource_int_attributes",
		},
		{
			level:  expression.LevelScope,
			column: "scope_int_attributes",
		},
		{
			level:       expression.LevelEvent,
			column:      "events.int_attributes",
			arrayColumn: "events",
		},
		{
			level:       expression.LevelLink,
			column:      "links.int_attributes",
			arrayColumn: "links",
		},
	}
	for _, tt := range tests {
		group := attributeGroup(tt.level, pcommon.ValueTypeInt)
		assert.Equal(t, tt.column, group.column(), tt.level)
		assert.Equal(t, tt.arrayColumn, group.arrayColumn(), tt.level)
		assert.Equal(t, "int_attributes", group.field(), tt.level)
	}
}

func TestNewAttributeGroup(t *testing.T) {
	group, err := NewAttributeGroup(expression.LevelScope, AttributeTypeComplex)
	require.NoError(t, err)
	assert.Equal(t, "scope_complex_attributes", group.column())

	_, err = NewAttributeGroup("trace", AttributeTypeStr)
	require.EqualError(t, err, `"trace" is not a level of attributes`)

	_, err = NewAttributeGroup(expression.LevelSpan, "text")
	require.EqualError(t, err, `"text" is not a type of attribute group`)
}
