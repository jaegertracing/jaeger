// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// attributeBuilder writes the conditions that search attributes, so that the way the spans
// table stores an attribute is decided here and not at each place that builds a query. A
// Reader owns one and hands it to every sqlBuilder it creates. The zero value writes
// conditions for the table that the backend creates.
//
// TODO: equality is the only attribute condition today. Two additions are expected:
//   - An ordering comparison (<, >=) is a condition like equality, so it adds an operator to
//     appendEquals. It keeps writing a whole condition, because an event or a link attribute
//     has one value per item and can be compared only inside the arrayExists lambda.
//   - Ordering a search by an attribute needs the attribute as a value. That is a second
//     method, which writes an expression with one value per span and therefore serves the
//     span, resource and scope levels only. Conditions must not be rebuilt on that method:
//     arrayExists matches every value of a key that a span carries more than once, and a
//     single-value expression does not.
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

// attributeGroup names the column group that stores the attributes of valueType at a level
// whose attributes are columns of the span row: span, resource or scope.
func attributeGroup(level expression.Level, valueType pcommon.ValueType) string {
	prefix, _ := levelColumns(level)
	if prefix != "" {
		prefix += "_"
	}
	return prefix + attributeColumnType(valueType) + "_attributes"
}

// appendEquals writes to q the condition that the attributes of one level hold the key and
// value of tav, and returns args extended with the values that the condition binds. It is the
// only place that writes the placeholders of an attribute condition, so they and their bind
// values cannot drift apart between callers.
func (attributeBuilder) appendEquals(q *sqlBuilder, indent int, args []any, level expression.Level, tav typedAttributeValue) []any {
	q.appendNewlineAndIndent(indent)
	if prefix, nested := levelColumns(level); nested {
		// Events and links are arrays inside the span row, so the condition searches the
		// attributes of every item.
		columnType := attributeColumnType(tav.valueType)
		q.WriteString("arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x." + columnType + "_attributes.key, x." + columnType + "_attributes.value), s." + prefix + ")")
	} else {
		group := attributeGroup(level, tav.valueType)
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + group + ".key, s." + group + ".value)")
	}
	return append(args, tav.key, tav.value)
}

// HoistedAttribute is one attribute that the spans table copies into a column of its own.
// The copy is a MATERIALIZED column that ClickHouse computes from the typed attribute group on
// insert, so the group stays the source of the value.
type HoistedAttribute struct {
	// Level is the span or the resource level.
	Level expression.Level
	// Key is the attribute key, an opaque string.
	Key string
	// Type is the stored type of the attribute: string, int, double or bool. An attribute
	// stored under the same key with another type is not copied.
	Type pcommon.ValueType
	// Column is the name of the dedicated column.
	Column string
}

// ColumnType returns the ClickHouse type of the dedicated column. The type is Nullable because
// a span without the attribute has no value, which is not the same as zero, false or an empty
// string.
func (h HoistedAttribute) ColumnType() string {
	switch h.Type {
	case pcommon.ValueTypeBool:
		return "Nullable(Bool)"
	case pcommon.ValueTypeDouble:
		return "Nullable(Float64)"
	case pcommon.ValueTypeInt:
		return "Nullable(Int64)"
	case pcommon.ValueTypeStr:
		return "Nullable(String)"
	default:
		// No other type can be hoisted, and an empty type makes the DDL fail.
		return ""
	}
}

// SourceExpression returns the SQL expression that the dedicated column is computed from: the
// value stored under Key in the typed group of Level, or NULL when the group has no such key.
func (h HoistedAttribute) SourceExpression() string {
	group := attributeGroup(h.Level, h.Type)
	key := quoteString(h.Key)
	return "if(has(" + group + ".key, " + key + "), " + group + ".value[indexOf(" + group + ".key, " + key + ")], NULL)"
}

// quoteString writes s as a ClickHouse string literal.
func quoteString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
