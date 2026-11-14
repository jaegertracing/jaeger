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
// conditions for a table in the nested layout without hoisted columns.
//
// TODO: equality is the only attribute condition today. Two additions are expected, and both
// read the hoisted column of an attribute that has one, as appendEquals does:
//   - An ordering comparison (<, >=) is a condition like equality, so it adds an operator to
//     appendEquals. It keeps writing a whole condition, because an event or a link attribute
//     has one value per item and can be compared only inside the arrayExists lambda.
//   - Ordering a search by an attribute needs the attribute as a value. That is a second
//     method, which writes an expression with one value per span and therefore serves the
//     span, resource and scope levels only. The condition of an attribute without a hoisted
//     column must not be rebuilt on that method: arrayExists matches every value of a key
//     that a span carries more than once, and a single-value expression does not.
type attributeBuilder struct {
	// layout is the layout of the typed attribute groups.
	layout AttributeLayout
	// columnBySource maps a hoisted attribute to the name of its column.
	columnBySource map[hoistedSource]string
}

// hoistedSource identifies a hoisted attribute. The type is part of the identity because the
// column copies one typed group only, so the same key stored with another type is not in it.
type hoistedSource struct {
	level     expression.Level
	key       string
	valueType pcommon.ValueType
}

func newAttributeBuilder(layout AttributeLayout, columns []HoistedColumn) attributeBuilder {
	builder := attributeBuilder{
		layout:         layout,
		columnBySource: make(map[hoistedSource]string, len(columns)),
	}
	for _, column := range columns {
		source := hoistedSource{
			level:     column.Level,
			key:       column.Key,
			valueType: column.Type,
		}
		builder.columnBySource[source] = column.Name
	}
	return builder
}

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
// values cannot drift apart between callers. A hoisted attribute is compared in its dedicated
// column; every other attribute, and the same key at another level or type, is searched in
// the typed groups.
func (b attributeBuilder) appendEquals(q *sqlBuilder, indent int, args []any, level expression.Level, tav typedAttributeValue) []any {
	q.appendNewlineAndIndent(indent)
	source := hoistedSource{
		level:     level,
		key:       tav.key,
		valueType: tav.valueType,
	}
	if name, ok := b.columnBySource[source]; ok {
		// The column is NULL on a span without the attribute, and NULL = ? is NULL, which
		// stays NULL under NOT and would drop the span. The IS NOT NULL test makes the
		// condition false there, as the array condition is false there. The column holds the
		// first value of a key that a span carries more than once, so a search for a later
		// value does not match, unlike the array condition.
		column := "s.`" + name + "`"
		q.WriteString("(" + column + " IS NOT NULL AND " + column + " = ?)")
		return append(args, tav.value)
	}
	// Events and links are arrays inside the span row, so their condition searches the
	// attributes of every item.
	prefix, nested := levelColumns(level)
	if b.layout == AttributeLayoutMap {
		// The subscript of a key that the map does not hold returns the default value of the
		// type, which would match a search for zero, false or an empty string, so the
		// condition tests that the key is there. The subscript returns the first value of a
		// key that a span carries more than once, so a search for a later value does not
		// match, unlike the array condition of the nested layout.
		if nested {
			group := "x." + attributeColumnType(tav.valueType) + "_attributes"
			q.WriteString("arrayExists(x -> mapContains(" + group + ", ?) AND " + group + "[?] = ?, s." + prefix + ")")
		} else {
			group := "s." + attributeGroup(level, tav.valueType)
			q.WriteString("(mapContains(" + group + ", ?) AND " + group + "[?] = ?)")
		}
		return append(args, tav.key, tav.key, tav.value)
	}
	if nested {
		columnType := attributeColumnType(tav.valueType)
		q.WriteString("arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x." + columnType + "_attributes.key, x." + columnType + "_attributes.value), s." + prefix + ")")
	} else {
		group := attributeGroup(level, tav.valueType)
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + group + ".key, s." + group + ".value)")
	}
	return append(args, tav.key, tav.value)
}
