// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"

// NestedLayout stores an attribute group as Nested(key String, value T), which is two
// parallel arrays.
type NestedLayout struct{}

var _ AttributeLayout = NestedLayout{}

func (NestedLayout) insertColumns(group string) []string {
	return []string{group + ".key", group + ".value"}
}

func (NestedLayout) appendRowAttributes(values []any, attributes dbmodel.Attributes) []any {
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

func (NestedLayout) appendItemAttributes(values []any, attributes dbmodel.Attributes2D) []any {
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
