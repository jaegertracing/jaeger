// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

func TestHoistedAttribute(t *testing.T) {
	tests := []struct {
		name       string
		attribute  HoistedAttribute
		columnType string
		expression string
	}{
		{
			name: "span string",
			attribute: HoistedAttribute{
				Level: expression.LevelSpan,
				Key:   "gen_ai.request.model",
				Type:  pcommon.ValueTypeStr,
			},
			columnType: "Nullable(String)",
			expression: "if(has(str_attributes.key, 'gen_ai.request.model'), str_attributes.value[indexOf(str_attributes.key, 'gen_ai.request.model')], NULL)",
		},
		{
			name: "resource int",
			attribute: HoistedAttribute{
				Level: expression.LevelResource,
				Key:   "k",
				Type:  pcommon.ValueTypeInt,
			},
			columnType: "Nullable(Int64)",
			expression: "if(has(resource_int_attributes.key, 'k'), resource_int_attributes.value[indexOf(resource_int_attributes.key, 'k')], NULL)",
		},
		{
			name: "span double",
			attribute: HoistedAttribute{
				Level: expression.LevelSpan,
				Key:   "k",
				Type:  pcommon.ValueTypeDouble,
			},
			columnType: "Nullable(Float64)",
			expression: "if(has(double_attributes.key, 'k'), double_attributes.value[indexOf(double_attributes.key, 'k')], NULL)",
		},
		{
			name: "span bool",
			attribute: HoistedAttribute{
				Level: expression.LevelSpan,
				Key:   "k",
				Type:  pcommon.ValueTypeBool,
			},
			columnType: "Nullable(Bool)",
			expression: "if(has(bool_attributes.key, 'k'), bool_attributes.value[indexOf(bool_attributes.key, 'k')], NULL)",
		},
		{
			name: "key with a quote and a backslash",
			attribute: HoistedAttribute{
				Level: expression.LevelSpan,
				Key:   `a'b\c`,
				Type:  pcommon.ValueTypeStr,
			},
			columnType: "Nullable(String)",
			expression: `if(has(str_attributes.key, 'a\'b\\c'), str_attributes.value[indexOf(str_attributes.key, 'a\'b\\c')], NULL)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.columnType, tt.attribute.ColumnType())
			assert.Equal(t, tt.expression, tt.attribute.SourceExpression())
		})
	}
}

func TestAttributeColumnType(t *testing.T) {
	for valueType, want := range map[pcommon.ValueType]string{
		pcommon.ValueTypeBool:   "bool",
		pcommon.ValueTypeDouble: "double",
		pcommon.ValueTypeInt:    "int",
		pcommon.ValueTypeStr:    "str",
		pcommon.ValueTypeBytes:  "complex",
		pcommon.ValueTypeMap:    "complex",
		pcommon.ValueTypeSlice:  "complex",
		pcommon.ValueTypeEmpty:  "",
	} {
		assert.Equal(t, want, attributeColumnType(valueType), valueType.String())
	}
}
