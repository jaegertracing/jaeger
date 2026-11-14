// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// NestedLayout stores an attribute group as Nested(key String, value T), which is two
// parallel arrays.
type NestedLayout struct{}

var _ AttributeLayout = NestedLayout{}

func (NestedLayout) insertSpan() string {
	return sql.InsertSpan
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

func (NestedLayout) selectSpans() string {
	return sql.SelectSpansQuery
}

// appendComparison searches the two arrays of the group for an entry with the key and a
// value that satisfies the comparison, so it matches any value of a key that a span carries
// more than once.
func (NestedLayout) appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any {
	entryMatches := "arrayExists((key, value) -> key = ? AND value " + sqlComparisonOperators[op] + " ?, "
	if prefix, nested := levelColumns(level); nested {
		// Events and links are arrays inside the span row, so the condition searches the
		// attributes of every event or link.
		group := "x." + attributeColumnType(tav.valueType) + "_attributes"
		q.WriteString("arrayExists(x -> " + entryMatches + group + ".key, " + group + ".value), s." + prefix + ")")
	} else {
		group := "s." + attributeGroup(level, tav.valueType)
		q.WriteString(entryMatches + group + ".key, " + group + ".value)")
	}
	return append(args, tav.key, tav.value)
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
