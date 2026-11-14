// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// NestedLayout stores an attribute group as Nested(key String, value T), which is two
// parallel arrays.
type NestedLayout struct{}

var _ AttributeLayout = NestedLayout{}

func (NestedLayout) Name() string {
	return "nested"
}

func (NestedLayout) GroupType(valueType string) string {
	return "Nested (key String, value " + valueType + ")"
}

// Declares accepts the two ways the server lists a Nested group: as the array of its keys,
// and as the group itself when flatten_nested is off.
func (NestedLayout) Declares(columnType string) bool {
	return strings.HasPrefix(columnType, "Array(") || strings.HasPrefix(columnType, "Nested(")
}

func (NestedLayout) hoistedSource(group string, key string) string {
	return "if(has(" + group + ".key, " + key + "), " + group + ".value[indexOf(" + group + ".key, " + key + ")], NULL)"
}

func (NestedLayout) columnNames(group string) []string {
	return []string{group + ".key", group + ".value"}
}

func (NestedLayout) appendAttributes(values []any, attributes dbmodel.Attributes) []any {
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

func (NestedLayout) appendAttributes2D(values []any, attributes dbmodel.Attributes2D) []any {
	return append(values,
		toTuple(attributes.BoolKeys, attributes.BoolValues),
		toTuple(attributes.DoubleKeys, attributes.DoubleValues),
		toTuple(attributes.IntKeys, attributes.IntValues),
		toTuple(attributes.StrKeys, attributes.StrValues),
		toTuple(attributes.ComplexKeys, attributes.ComplexValues),
	)
}

func (NestedLayout) Keys(group string) string {
	return group + ".key"
}

func (NestedLayout) values(group string) string {
	return group + ".value"
}

func (NestedLayout) Keys2D(group string) string {
	return group + ".key"
}

func (NestedLayout) values2D(group string) string {
	return group + ".value"
}

// appendEquals searches the two arrays of the group for an entry with the key and the value,
// so it matches any value of a key that a span carries more than once.
func (NestedLayout) appendEquals(q *sqlBuilder, args []any, level expression.Level, tav typedAttributeValue) []any {
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
