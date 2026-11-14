// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

type Writer struct {
	conn       driver.Conn
	layout     AttributeLayout
	insertSpan string
}

// NewWriter returns a new Writer instance that uses the given ClickHouse connection
// to write trace data into a spans table of the given attribute layout.
//
// The provided connection is used for writing traces.
// This connection should not have instrumentation enabled to avoid recursively generating traces.
func NewWriter(conn driver.Conn, layout AttributeLayout) *Writer {
	return &Writer{
		conn:       conn,
		layout:     layout,
		insertSpan: layout.insertSpan(),
	}
}

func (w *Writer) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	batch, err := w.conn.PrepareBatch(ctx, w.insertSpan)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}
	defer batch.Close()
	for _, rs := range td.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				sr := dbmodel.ToRow(rs.Resource(), ss.Scope(), span)
				// The values follow the order of spanColumns.
				values := []any{
					sr.ID,
					sr.TraceID,
					sr.TraceState,
					sr.ParentSpanID,
					sr.Name,
					sr.Kind,
					sr.StartTime,
					sr.StatusCode,
					sr.StatusMessage,
					sr.Duration,
				}
				values = w.appendRowAttributes(values, sr.Attributes)
				values = append(values, sr.EventNames, sr.EventTimestamps)
				values = w.appendItemAttributes(values, sr.EventAttributes)
				values = append(values, sr.LinkTraceIDs, sr.LinkSpanIDs, sr.LinkTraceStates)
				values = w.appendItemAttributes(values, sr.LinkAttributes)
				values = append(values, sr.ServiceName)
				values = w.appendRowAttributes(values, sr.ResourceAttributes)
				values = append(values, sr.ScopeName, sr.ScopeVersion)
				values = w.appendRowAttributes(values, sr.ScopeAttributes)
				if err = batch.Append(values...); err != nil {
					return fmt.Errorf("failed to append span to batch: %w", err)
				}
			}
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	return nil
}

// appendRowAttributes appends the values that the insert statement binds for the attribute
// groups of the span row.
func (w *Writer) appendRowAttributes(values []any, attributes dbmodel.Attributes) []any {
	values = appendRowGroup(w.layout, values, attributes.BoolKeys, attributes.BoolValues)
	values = appendRowGroup(w.layout, values, attributes.DoubleKeys, attributes.DoubleValues)
	values = appendRowGroup(w.layout, values, attributes.IntKeys, attributes.IntValues)
	values = appendRowGroup(w.layout, values, attributes.StrKeys, attributes.StrValues)
	return appendRowGroup(w.layout, values, attributes.ComplexKeys, attributes.ComplexValues)
}

// appendItemAttributes appends the values that the insert statement binds for the attribute
// groups of the events or of the links, where keys[i] and values[i] belong to item i.
func (w *Writer) appendItemAttributes(values []any, attributes dbmodel.Attributes2D) []any {
	values = appendItemGroup(w.layout, values, attributes.BoolKeys, attributes.BoolValues)
	values = appendItemGroup(w.layout, values, attributes.DoubleKeys, attributes.DoubleValues)
	values = appendItemGroup(w.layout, values, attributes.IntKeys, attributes.IntValues)
	values = appendItemGroup(w.layout, values, attributes.StrKeys, attributes.StrValues)
	return appendItemGroup(w.layout, values, attributes.ComplexKeys, attributes.ComplexValues)
}

func appendRowGroup[T any](layout AttributeLayout, values []any, keys []string, groupValues []T) []any {
	if layout == AttributeLayoutMap {
		return append(values, orderedMap[T]{keys: keys, values: groupValues})
	}
	return append(values, keys, groupValues)
}

func appendItemGroup[T any](layout AttributeLayout, values []any, keys [][]string, groupValues [][]T) []any {
	if layout != AttributeLayoutMap {
		return append(values, toTuple(keys, groupValues))
	}
	maps := make([]orderedMap[T], 0, len(keys))
	for i := range keys {
		maps = append(maps, orderedMap[T]{keys: keys[i], values: groupValues[i]})
	}
	return append(values, maps)
}

// orderedMap hands the keys and values of one attribute group to the ClickHouse driver as
// the value of a Map column. A Go map cannot do that, because it keeps neither the order of
// the keys nor a key that the span carries more than once.
type orderedMap[T any] struct {
	keys   []string
	values []T
}

var _ column.IterableOrderedMap = orderedMap[string]{}

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
