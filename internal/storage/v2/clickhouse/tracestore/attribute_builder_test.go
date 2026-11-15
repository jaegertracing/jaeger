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
	nestedSelectSpans          = sql.SelectSpansNestedLayout
	nestedSelectSpansByTraceID = nestedSelectSpans + " WHERE s.trace_id = ?"
)

// newNestedSQLBuilder returns a builder for a table in the nested layout without hoisted
// columns.
func newNestedSQLBuilder() sqlBuilder {
	return sqlBuilder{attrBuilder: newAttributeBuilder(NestedLayout{}, nil)}
}

// TestAttributeBuilder_AppendComparison snapshots, in one file per layout, the condition for
// every level and comparison below. The span attribute "model" of type string and the
// resource attribute "shard" of type int are hoisted. Each condition is followed by the
// values that it binds.
func TestAttributeBuilder_AppendComparison(t *testing.T) {
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
			op: expression.OpEq,
			tav: typedAttributeValue{
				key:       "model",
				value:     int64(1),
				valueType: pcommon.ValueTypeInt,
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
	layouts := []struct {
		name   string
		layout AttributeLayout
	}{
		{
			name:   "nested layout",
			layout: NestedLayout{},
		},
		{
			name:   "map layout",
			layout: MapLayout{},
		},
	}
	for _, tt := range layouts {
		t.Run(tt.name, func(t *testing.T) {
			builder := newAttributeBuilder(tt.layout, testHoistedColumns)
			var q sqlBuilder
			for _, level := range levels {
				for _, c := range comparisons {
					fmt.Fprintf(&q, "\n-- %s attribute %q of type %s", level, c.tav.key, c.tav.valueType)
					args := builder.appendComparison(&q, 0, nil, level, c.op, c.tav)
					fmt.Fprintf(&q, " -- binds %v", args)
				}
			}
			clickhousetest.VerifyQuerySnapshot(t, q.String())
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
