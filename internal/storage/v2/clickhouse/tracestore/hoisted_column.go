// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// hoistedColumnTypes maps the stored type of an attribute to the type of its hoisted column.
// The type is Nullable because a span without the attribute has no value, which is not the
// same as zero, false or an empty string.
var hoistedColumnTypes = map[pcommon.ValueType]string{
	pcommon.ValueTypeBool:   "Nullable(Bool)",
	pcommon.ValueTypeDouble: "Nullable(Float64)",
	pcommon.ValueTypeInt:    "Nullable(Int64)",
	pcommon.ValueTypeStr:    "Nullable(String)",
}

// HoistedColumn is a column of the spans table that holds a copy of one attribute. ClickHouse
// computes it from the typed attribute group on insert, so the group stays the source of the
// value.
type HoistedColumn struct {
	// Name is the name of the column.
	Name string
	// Level is the level of the attribute: span or resource.
	Level expression.Level
	// Key is the attribute key, an opaque string.
	Key string
	// Type is the stored type of the attribute: string, int, double or bool. An attribute
	// stored under the same key with another type is not copied.
	Type pcommon.ValueType
	// Codec is the compression codec of the column.
	Codec string
	// Layout is the layout of the typed group that the column copies from.
	Layout AttributeLayout
}

// Definition returns the column as CREATE TABLE declares it. The name is quoted because a
// name such as index is a keyword at that position.
func (c HoistedColumn) Definition() string {
	return "`" + c.Name + "` " + c.Declaration()
}

// Declaration returns the type, expression and codec of the column, as they follow its name
// in CREATE TABLE.
func (c HoistedColumn) Declaration() string {
	return c.NullableType() + " MATERIALIZED " + c.SourceExpression() + " CODEC(" + c.Codec + ")"
}

// NullableType returns the ClickHouse type of the column.
func (c HoistedColumn) NullableType() string {
	return hoistedColumnTypes[c.Type]
}

// SourceExpression returns the expression that ClickHouse computes the column from: the value
// stored under Key in the typed group of Level, or NULL when the group has no such key.
func (c HoistedColumn) SourceExpression() string {
	group := attributeGroup(c.Level, c.Type)
	key := quoteString(c.Key)
	if c.Layout == AttributeLayoutMap {
		return "if(mapContains(" + group + ", " + key + "), " + group + "[" + key + "], NULL)"
	}
	return "if(has(" + group + ".key, " + key + "), " + group + ".value[indexOf(" + group + ".key, " + key + ")], NULL)"
}

// quoteString returns s as a ClickHouse string literal.
func quoteString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
