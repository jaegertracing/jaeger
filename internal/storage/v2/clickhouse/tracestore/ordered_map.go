// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import "github.com/ClickHouse/clickhouse-go/v2/lib/column"

var _ column.IterableOrderedMap = orderedMap[string]{}

// orderedMap is the value of a Map column that the writer gives to the ClickHouse driver in place
// of a Go map, which can neither hold a repeated attribute key nor keep the keys in span order.
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
