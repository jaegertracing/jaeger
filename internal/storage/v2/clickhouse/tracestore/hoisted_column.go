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
}

// Definition returns the column as CREATE TABLE declares it. The name is quoted because a
// name such as index is a keyword at that position.
func (c HoistedColumn) Definition() string {
	return "`" + c.Name + "` " + hoistedColumnTypes[c.Type] + " MATERIALIZED " + c.sourceExpression() + " CODEC(" + c.Codec + ")"
}

// sourceExpression returns the expression that ClickHouse computes the column from: the value
// stored under Key in the typed group of Level, or NULL when the group has no such key.
func (c HoistedColumn) sourceExpression() string {
	group := attributeGroup(c.Level, c.Type)
	key := quoteString(c.Key)
	return "if(has(" + group + ".key, " + key + "), " + group + ".value[indexOf(" + group + ".key, " + key + ")], NULL)"
}

// quoteString returns s as a ClickHouse string literal.
func quoteString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
