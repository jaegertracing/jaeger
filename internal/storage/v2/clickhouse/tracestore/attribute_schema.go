// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// attributeSchema describes how the spans table stores attributes and writes the conditions
// that search them. A Reader owns one and hands it to every sqlBuilder it creates, so a
// storage decision that depends on the backend's configuration is made here and not at the
// places that build a query. The zero value describes the table that the backend creates.
type attributeSchema struct{}

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

// attributeLocation is where a level's attributes live: the column prefix of that level, and
// whether the level is a nested array (event, link) rather than a flat one (span, resource,
// scope).
func attributeLocation(level expression.Level) (prefix string, nested bool) {
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
func (attributeSchema) appendEquals(q *sqlBuilder, indent int, args []any, level expression.Level, tav typedAttributeValue) []any {
	columnType := attributeColumnType(tav.valueType)
	prefix, nested := attributeLocation(level)
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
