// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ AttributeLayout = MapLayout{}

// MapLayout stores an attribute group as Map(String, T).
type MapLayout struct{}

func (MapLayout) Name() string {
	return "map"
}

func (MapLayout) ColumnTypeSQL(attributeType AttributeType) string {
	return "Map(String, " + attributeType.valueTypeSQL() + ")"
}

func (MapLayout) Declares(columnType string) bool {
	return strings.HasPrefix(columnType, "Map(")
}

func (MapLayout) insertSpanSQL() string {
	return sql.InsertSpanMapLayout
}

func (MapLayout) selectSpansSQL() string {
	return sql.SelectSpansMapLayout
}

func (MapLayout) KeysSQL(group AttributeGroup) string {
	if group.arrayColumn() != "" {
		return "arrayMap(m -> mapKeys(m), " + group.column() + ")"
	}
	return "mapKeys(" + group.column() + ")"
}

func (MapLayout) valueSQL(group AttributeGroup, key string) string {
	g := group.column()
	return "if(mapContains(" + g + ", " + key + "), " + g + "[" + key + "], NULL)"
}

// appendComparison compares the value that the map holds under the key. The subscript of a
// key that the map does not hold returns the default value of the type, which would match a
// search for zero, false or an empty string, so the condition tests that the key is there.
// The subscript returns the first value of a key that a span carries more than once, so a
// search for a later value does not match, unlike in the nested layout.
func (MapLayout) appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any {
	comparison := "[?] " + sqlComparisonOperators[op] + " ?"
	group := attributeGroup(level, tav.valueType)
	if arrayColumn := group.arrayColumn(); arrayColumn != "" {
		// Events and links are arrays inside the span row, so the condition searches the
		// attributes of every event or link.
		column := "x." + group.field()
		q.WriteString("arrayExists(x -> mapContains(" + column + ", ?) AND " + column + comparison + ", s." + arrayColumn + ")")
	} else {
		column := "s." + group.column()
		q.WriteString("(mapContains(" + column + ", ?) AND " + column + comparison + ")")
	}
	return append(args, tav.key, tav.key, tav.value)
}

func (MapLayout) appendHasKey(q *sqlBuilder, args []any, group AttributeGroup, key string) []any {
	q.WriteString("mapContains(" + group.operandSQL() + ", ?)")
	return append(args, key)
}

func (MapLayout) bindAttributes(values []any, attributes dbmodel.Attributes) []any {
	return append(values,
		orderedMap[bool]{keys: attributes.BoolKeys, values: attributes.BoolValues},
		orderedMap[float64]{keys: attributes.DoubleKeys, values: attributes.DoubleValues},
		orderedMap[int64]{keys: attributes.IntKeys, values: attributes.IntValues},
		orderedMap[string]{keys: attributes.StrKeys, values: attributes.StrValues},
		orderedMap[string]{keys: attributes.ComplexKeys, values: attributes.ComplexValues},
	)
}

func (MapLayout) bindAttributes2D(values []any, attributes dbmodel.Attributes2D) []any {
	return append(values,
		orderedMaps(attributes.BoolKeys, attributes.BoolValues),
		orderedMaps(attributes.DoubleKeys, attributes.DoubleValues),
		orderedMaps(attributes.IntKeys, attributes.IntValues),
		orderedMaps(attributes.StrKeys, attributes.StrValues),
		orderedMaps(attributes.ComplexKeys, attributes.ComplexValues),
	)
}

// orderedMaps returns one orderedMap per item, where keys[i] and values[i] belong to item i.
func orderedMaps[T any](keys [][]string, values [][]T) []orderedMap[T] {
	maps := make([]orderedMap[T], 0, len(keys))
	for i := range keys {
		maps = append(maps, orderedMap[T]{keys: keys[i], values: values[i]})
	}
	return maps
}
