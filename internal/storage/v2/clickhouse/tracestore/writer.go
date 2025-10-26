// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

type Writer struct {
	conn driver.Conn
}

// NewWriter returns a new Writer instance that uses the given ClickHouse connection
// to write trace data.
//
// The provided connection is used for writing traces.
// This connection should not have instrumentation enabled to avoid recursively generating traces.
func NewWriter(conn driver.Conn) *Writer {
	return &Writer{conn: conn}
}

func (w *Writer) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	batch, err := w.conn.PrepareBatch(ctx, sql.InsertSpan)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}
	defer batch.Close()
	for _, rs := range td.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				sr := dbmodel.ToRow(rs.Resource(), ss.Scope(), span)
				err = batch.Append(
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
					sr.Attributes.BoolKeys,
					sr.Attributes.BoolValues,
					sr.Attributes.DoubleKeys,
					sr.Attributes.DoubleValues,
					sr.Attributes.IntKeys,
					sr.Attributes.IntValues,
					sr.Attributes.StrKeys,
					sr.Attributes.StrValues,
					sr.Attributes.ComplexKeys,
					sr.Attributes.ComplexValues,
					sr.EventNames,
					sr.EventTimestamps,
					toTuple(sr.EventAttributes.BoolKeys, sr.EventAttributes.BoolValues),
					toTuple(sr.EventAttributes.DoubleKeys, sr.EventAttributes.DoubleValues),
					toTuple(sr.EventAttributes.IntKeys, sr.EventAttributes.IntValues),
					toTuple(sr.EventAttributes.StrKeys, sr.EventAttributes.StrValues),
					toTuple(sr.EventAttributes.ComplexKeys, sr.EventAttributes.ComplexValues),
					sr.LinkTraceIDs,
					sr.LinkSpanIDs,
					sr.LinkTraceStates,
					toTuple(sr.LinkAttributes.BoolKeys, sr.LinkAttributes.BoolValues),
					toTuple(sr.LinkAttributes.DoubleKeys, sr.LinkAttributes.DoubleValues),
					toTuple(sr.LinkAttributes.IntKeys, sr.LinkAttributes.IntValues),
					toTuple(sr.LinkAttributes.StrKeys, sr.LinkAttributes.StrValues),
					toTuple(sr.LinkAttributes.ComplexKeys, sr.LinkAttributes.ComplexValues),
					sr.ServiceName,
					sr.ResourceAttributes.BoolKeys,
					sr.ResourceAttributes.BoolValues,
					sr.ResourceAttributes.DoubleKeys,
					sr.ResourceAttributes.DoubleValues,
					sr.ResourceAttributes.IntKeys,
					sr.ResourceAttributes.IntValues,
					sr.ResourceAttributes.StrKeys,
					sr.ResourceAttributes.StrValues,
					sr.ResourceAttributes.ComplexKeys,
					sr.ResourceAttributes.ComplexValues,
					sr.ScopeName,
					sr.ScopeVersion,
					sr.ScopeAttributes.BoolKeys,
					sr.ScopeAttributes.BoolValues,
					sr.ScopeAttributes.DoubleKeys,
					sr.ScopeAttributes.DoubleValues,
					sr.ScopeAttributes.IntKeys,
					sr.ScopeAttributes.IntValues,
					sr.ScopeAttributes.StrKeys,
					sr.ScopeAttributes.StrValues,
					sr.ScopeAttributes.ComplexKeys,
					sr.ScopeAttributes.ComplexValues,
				)
				if err != nil {
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
