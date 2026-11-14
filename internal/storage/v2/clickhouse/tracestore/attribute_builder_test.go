// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// testHoistedColumns hoists the string span attribute "model" and the int resource attribute
// "shard".
var testHoistedColumns = []HoistedColumn{
	{
		Name:  "model_column",
		Level: expression.LevelSpan,
		Key:   "model",
		Type:  pcommon.ValueTypeStr,
	},
	{
		Name:  "shard_column",
		Level: expression.LevelResource,
		Key:   "shard",
		Type:  pcommon.ValueTypeInt,
	},
}

// newHoistedColumnReader returns a reader of a table with testHoistedColumns, whose attribute
// metadata says that the resource attribute "shard" is stored as an int.
func newHoistedColumnReader() *Reader {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{
							AttributeKey: "shard",
							Type:         "int",
							Level:        "resource",
						},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	config := testReaderConfig
	config.HoistedColumns = testHoistedColumns
	return NewReader(driver, NestedLayout{}, config)
}

// nestedSelectSpans and nestedSelectSpansByTraceID are the statements that read spans from
// a table in the nested layout, which the test driver matches its responses by.
var (
	nestedSelectSpans          = sql.SelectSpansQuery
	nestedSelectSpansByTraceID = nestedSelectSpans + " WHERE s.trace_id = ?"
)

// newNestedSQLBuilder returns a builder for a table in the nested layout without hoisted
// columns.
func newNestedSQLBuilder() sqlBuilder {
	return sqlBuilder{attrBuilder: newAttributeBuilder(NestedLayout{}, nil)}
}

func TestAttributeBuilder_AppendComparison(t *testing.T) {
	tests := []struct {
		name           string
		hoistedColumns []HoistedColumn
		level          expression.Level
		op             expression.Operator
		tav            typedAttributeValue
		wantArgs       []any
	}{
		{
			name:           "hoisted span attribute",
			hoistedColumns: testHoistedColumns,
			level:          expression.LevelSpan,
			tav: typedAttributeValue{
				key:       "model",
				value:     "x",
				valueType: pcommon.ValueTypeStr,
			},
			wantArgs: []any{"x"},
		},
		{
			name:           "hoisted resource attribute",
			hoistedColumns: testHoistedColumns,
			level:          expression.LevelResource,
			tav: typedAttributeValue{
				key:       "shard",
				value:     int64(0),
				valueType: pcommon.ValueTypeInt,
			},
			wantArgs: []any{int64(0)},
		},
		{
			name:           "hoisted key stored with another type",
			hoistedColumns: testHoistedColumns,
			level:          expression.LevelSpan,
			tav: typedAttributeValue{
				key:       "model",
				value:     int64(1),
				valueType: pcommon.ValueTypeInt,
			},
			wantArgs: []any{"model", int64(1)},
		},
		{
			name:           "hoisted key at another level",
			hoistedColumns: testHoistedColumns,
			level:          expression.LevelResource,
			tav: typedAttributeValue{
				key:       "model",
				value:     "x",
				valueType: pcommon.ValueTypeStr,
			},
			wantArgs: []any{"model", "x"},
		},
		{
			name:           "hoisted key in an event",
			hoistedColumns: testHoistedColumns,
			level:          expression.LevelEvent,
			tav: typedAttributeValue{
				key:       "model",
				value:     "x",
				valueType: pcommon.ValueTypeStr,
			},
			wantArgs: []any{"model", "x"},
		},
		{
			name:           "hoisted attribute below a value",
			hoistedColumns: testHoistedColumns,
			level:          expression.LevelResource,
			op:             expression.OpLt,
			tav: typedAttributeValue{
				key:       "shard",
				value:     int64(8),
				valueType: pcommon.ValueTypeInt,
			},
			wantArgs: []any{int64(8)},
		},
		{
			name:  "event attribute at or above a value",
			level: expression.LevelEvent,
			op:    expression.OpGte,
			tav: typedAttributeValue{
				key:       "ratio",
				value:     0.5,
				valueType: pcommon.ValueTypeDouble,
			},
			wantArgs: []any{"ratio", 0.5},
		},
		{
			name:  "no hoisted columns",
			level: expression.LevelSpan,
			tav: typedAttributeValue{
				key:       "model",
				value:     "x",
				valueType: pcommon.ValueTypeStr,
			},
			wantArgs: []any{"model", "x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := tt.op
			if op == "" {
				op = expression.OpEq
			}
			var q sqlBuilder
			args := newAttributeBuilder(NestedLayout{}, tt.hoistedColumns).appendComparison(&q, 0, nil, tt.level, op, tt.tav)
			clickhousetest.VerifyQuerySnapshot(t, q.String())
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestAttributeBuilder_AppendComparison_MapLayout snapshots the condition for every level
// and value type in one file. The span attribute "model" and the resource attribute "shard"
// are hoisted, and each condition is followed by the values that it binds.
func TestAttributeBuilder_AppendComparison_MapLayout(t *testing.T) {
	levels := []expression.Level{
		expression.LevelSpan,
		expression.LevelResource,
		expression.LevelScope,
		expression.LevelEvent,
		expression.LevelLink,
	}
	comparisons := []struct {
		op  expression.Operator
		tav typedAttributeValue
	}{
		{
			op: expression.OpEq,
			tav: typedAttributeValue{
				key:       "model",
				value:     "x",
				valueType: pcommon.ValueTypeStr,
			},
		},
		{
			op: expression.OpGte,
			tav: typedAttributeValue{
				key:       "shard",
				value:     int64(0),
				valueType: pcommon.ValueTypeInt,
			},
		},
		{
			op: expression.OpLt,
			tav: typedAttributeValue{
				key:       "ratio",
				value:     0.5,
				valueType: pcommon.ValueTypeDouble,
			},
		},
		{
			op: expression.OpEq,
			tav: typedAttributeValue{
				key:       "retry",
				value:     true,
				valueType: pcommon.ValueTypeBool,
			},
		},
	}
	builder := newAttributeBuilder(MapLayout{}, testHoistedColumns)
	var q sqlBuilder
	for _, level := range levels {
		for _, c := range comparisons {
			fmt.Fprintf(&q, "\n-- %s attribute %q", level, c.tav.key)
			args := builder.appendComparison(&q, 0, nil, level, c.op, c.tav)
			fmt.Fprintf(&q, " -- binds %v", args)
		}
	}
	clickhousetest.VerifyQuerySnapshot(t, q.String())
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
