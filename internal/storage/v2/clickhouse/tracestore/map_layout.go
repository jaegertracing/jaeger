// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"github.com/ClickHouse/clickhouse-go/v2/lib/column"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var (
	_ AttributeLayout           = MapLayout{}
	_ column.IterableOrderedMap = orderedMap[string]{}
)

// MapLayout stores an attribute group as Map(String, T).
type MapLayout struct{}

func (MapLayout) insertSpanSQL() string {
	return sql.InsertSpanMapLayout
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

func (MapLayout) selectSpansSQL() string {
	return sql.SelectSpansMapLayout
}

// appendComparison compares the value that the map holds under the key. The subscript of a
// key that the map does not hold returns the default value of the type, which would match a
// search for zero, false or an empty string, so the condition tests that the key is there.
// The subscript returns the first value of a key that a span carries more than once, so a
// search for a later value does not match, unlike in the nested layout.
func (MapLayout) appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any {
	comparison := "[?] " + sqlComparisonOperators[op] + " ?"
	if prefix, nested := levelColumns(level); nested {
		// Events and links are arrays inside the span row, so the condition searches the
		// attributes of every event or link.
		group := "x." + attributeColumnType(tav.valueType) + "_attributes"
		q.WriteString("arrayExists(x -> mapContains(" + group + ", ?) AND " + group + comparison + ", s." + prefix + ")")
	} else {
		group := "s." + attributeGroup(level, tav.valueType)
		q.WriteString("(mapContains(" + group + ", ?) AND " + group + comparison + ")")
	}
	return append(args, tav.key, tav.key, tav.value)
}

// orderedMaps returns one orderedMap per item, where keys[i] and values[i] belong to item i.
func orderedMaps[T any](keys [][]string, values [][]T) []orderedMap[T] {
	maps := make([]orderedMap[T], 0, len(keys))
	for i := range keys {
		maps = append(maps, orderedMap[T]{keys: keys[i], values: values[i]})
	}
	return maps
}

// orderedMap hands the keys and values of one attribute group to the ClickHouse driver as
// the value of a Map column. A Go map cannot do that, because it keeps neither the order of
// the keys nor a key that the span carries more than once.
type orderedMap[T any] struct {
	keys   []string
	values []T
}

// Put is part of column.IterableOrderedMap, which the driver calls only when it scans a
// row into the map. The writer never scans, so Put stores nothing.
func (orderedMap[T]) Put(any, any) {}

func (m orderedMap[T]) Iterator() column.MapIterator {
	return &orderedMapIterator[T]{m: m, next: 0}
}

// orderedMapIterator walks an orderedMap. Key and Value return the entry that the last call
// of Next moved to.
type orderedMapIterator[T any] struct {
	m    orderedMap[T]
	next int
}

func (it *orderedMapIterator[T]) Next() bool {
	it.next++
	return it.next <= len(it.m.keys)
}

func (it *orderedMapIterator[T]) Key() any {
	return it.m.keys[it.next-1]
}

func (it *orderedMapIterator[T]) Value() any {
	return it.m.values[it.next-1]
}
