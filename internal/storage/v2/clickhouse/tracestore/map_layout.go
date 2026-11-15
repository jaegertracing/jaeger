// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ AttributeLayout = MapLayout{}

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

// orderedMaps returns one orderedMap per item, where keys[i] and values[i] belong to item i.
func orderedMaps[T any](keys [][]string, values [][]T) []orderedMap[T] {
	maps := make([]orderedMap[T], 0, len(keys))
	for i := range keys {
		maps = append(maps, orderedMap[T]{keys: keys[i], values: values[i]})
	}
	return maps
}
