// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ AttributeLayout = NestedLayout{}

// NestedLayout stores an attribute group as Nested(key String, value T), which is two
// parallel arrays.
type NestedLayout struct{}

func (NestedLayout) Name() string {
	return "nested"
}

func (NestedLayout) ColumnTypeSQL(attributeType AttributeType) string {
	return "Nested (key String, value " + attributeType.valueTypeSQL() + ")"
}

func (NestedLayout) Declares(columnType string) bool {
	return strings.HasPrefix(columnType, "Array(") || strings.HasPrefix(columnType, "Nested(")
}

func (NestedLayout) insertSpanSQL() string {
	return sql.InsertSpanNestedLayout
}

func (NestedLayout) selectSpansSQL() string {
	return sql.SelectSpansNestedLayout
}

func (NestedLayout) KeysSQL(group AttributeGroup) string {
	return group.column() + ".key"
}

func (NestedLayout) valueSQL(group AttributeGroup, key string) string {
	g := group.column()
	return "if(has(" + g + ".key, " + key + "), " + g + ".value[indexOf(" + g + ".key, " + key + ")], NULL)"
}

// appendComparison searches the two arrays of the group for an entry with the key and a
// value that satisfies the comparison, so it matches any value of a key that a span carries
// more than once.
func (NestedLayout) appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any {
	entryMatches := "arrayExists((key, value) -> key = ? AND value " + sqlComparisonOperators[op] + " ?, "
	group := attributeGroup(level, tav.valueType)
	column := group.operandSQL()
	if arrayColumn := q.arrayColumnToSearch(level); arrayColumn != "" {
		// Events and links are arrays inside the span row, so the condition searches the
		// attributes of every event or link.
		q.WriteString("arrayExists(x -> " + entryMatches + column + ".key, " + column + ".value), s." + arrayColumn + ")")
	} else {
		q.WriteString(entryMatches + column + ".key, " + column + ".value)")
	}
	return append(args, tav.key, tav.value)
}

func (NestedLayout) appendHasKey(q *sqlBuilder, args []any, group AttributeGroup, key string) []any {
	q.WriteString("has(" + group.operandSQL() + ".key, ?)")
	return append(args, key)
}

func (NestedLayout) bindAttributes(values []any, attributes dbmodel.Attributes) []any {
	return append(values,
		attributes.BoolKeys,
		attributes.BoolValues,
		attributes.DoubleKeys,
		attributes.DoubleValues,
		attributes.IntKeys,
		attributes.IntValues,
		attributes.StrKeys,
		attributes.StrValues,
		attributes.ComplexKeys,
		attributes.ComplexValues,
	)
}

func (NestedLayout) bindAttributes2D(values []any, attributes dbmodel.Attributes2D) []any {
	return append(values,
		toTuple(attributes.BoolKeys, attributes.BoolValues),
		toTuple(attributes.DoubleKeys, attributes.DoubleValues),
		toTuple(attributes.IntKeys, attributes.IntValues),
		toTuple(attributes.StrKeys, attributes.StrValues),
		toTuple(attributes.ComplexKeys, attributes.ComplexValues),
	)
}

func toTuple[T any](keys [][]string, values [][]T) [][][]any {
	tuple := make([][][]any, 0, len(keys))
	for i := range keys {
		inner := make([][]any, 0, len(keys[i]))
		for j := range keys[i] {
			inner = append(inner, []any{keys[i][j], values[i][j]})
		}
		tuple = append(tuple, inner)
	}
	return tuple
}
