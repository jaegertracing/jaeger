// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// attributeBuilder writes the conditions that search attributes, so that the way the spans
// table stores an attribute is decided here and not at each place that builds a query. A
// Reader owns one and hands it to every sqlBuilder it creates. The zero value writes
// conditions for the table that the backend creates.
type attributeBuilder struct{}

// attributeColumnType names the typed column group that stores values of valueType. Bytes, maps
// and slices share the complex group, where the key carries a prefix that tells them apart.
// The names are spelled out because they are column names of the spans table, which must not
// change when a library changes how it prints a value type.
func attributeColumnType(valueType pcommon.ValueType) string {
	switch valueType {
	case pcommon.ValueTypeBool:
		return "bool"
	case pcommon.ValueTypeDouble:
		return "double"
	case pcommon.ValueTypeInt:
		return "int"
	case pcommon.ValueTypeStr:
		return "str"
	case pcommon.ValueTypeBytes, pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		return "complex"
	default:
		// No column group stores an empty value, and no caller passes one.
		return ""
	}
}

// levelColumns returns the column prefix of a level, and whether the level is a nested array
// (event, link) rather than a set of columns of the span row (span, resource, scope).
func levelColumns(level expression.Level) (prefix string, nested bool) {
	switch level {
	case expression.LevelResource:
		return "resource", false
	case expression.LevelScope:
		return "scope", false
	case expression.LevelEvent:
		return "events", true
	case expression.LevelLink:
		return "links", true
	default: // expression.LevelSpan
		return "", false
	}
}

// appendEquals writes to q the condition that the attributes of one level hold the key and
// value of tav, and returns args extended with the values that the condition binds. It is the
// only place that writes the placeholders of an attribute condition, so they and their bind
// values cannot drift apart between callers.
func (attributeBuilder) appendEquals(q *sqlBuilder, indent int, args []any, level expression.Level, tav typedAttributeValue) []any {
	columnType := attributeColumnType(tav.valueType)
	prefix, nested := levelColumns(level)
	q.appendNewlineAndIndent(indent)
	if nested {
		// Events and links are arrays inside the span row, so the condition searches the
		// attributes of every item.
		q.WriteString("arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x." + columnType + "_attributes.key, x." + columnType + "_attributes.value), s." + prefix + ")")
	} else {
		if prefix != "" {
			prefix += "_"
		}
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + prefix + columnType + "_attributes.key, s." + prefix + columnType + "_attributes.value)")
	}
	return append(args, tav.key, tav.value)
}
