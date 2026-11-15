// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// attributeBuilder writes the conditions that search attributes, so that the way the spans
// table stores an attribute is decided here and not at each place that builds a query. The
// zero value is not usable, because every condition needs the layout.
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

// appendComparison writes to q the condition that an attribute of the given level has the
// key of tav and a value that compares to the value of tav as op says, and returns args
// extended with the values that the condition binds. It is the only place that writes the
// placeholders of an attribute condition, so they and their bind values cannot drift apart
// between callers. A hoisted attribute is compared in its dedicated column; every other
// attribute, and the same key at another level or type, is searched in the typed groups by
// the condition of the layout. The condition is a whole one for every level, because an
// event or a link attribute has one value per event or link and can be compared only
// inside a lambda.
func (b attributeBuilder) appendComparison(
	q *sqlBuilder,
	indent int,
	args []any,
	level expression.Level,
	op expression.Operator,
	tav typedAttributeValue,
) []any {
	q.appendNewlineAndIndent(indent)
	source := hoistedSource{
		level:     level,
		key:       tav.key,
		valueType: tav.valueType,
	}
	if name, ok := b.columnBySource[source]; ok {
		// The column is NULL on a span without the attribute, and a comparison with NULL is
		// NULL, which stays NULL under NOT and would drop the span. The IS NOT NULL test
		// makes the condition false there, as the condition of the layout is false there.
		// The column holds the first value of a key that a span carries more than once, so a
		// search for a later value does not match, unlike the condition of the nested layout.
		column := "s.`" + name + "`"
		q.WriteString("(" + column + " IS NOT NULL AND " + column + " " + sqlComparisonOperators[op] + " ?)")
		return append(args, tav.value)
	}
	return b.layout.appendComparison(q, args, level, op, tav)
}
