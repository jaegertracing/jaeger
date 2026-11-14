// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

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
		insertSpan: insertSpan(layout),
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
				values = w.layout.appendAttributes(values, sr.Attributes)
				values = append(values, sr.EventNames, sr.EventTimestamps)
				values = w.layout.appendAttributes2D(values, sr.EventAttributes)
				values = append(values, sr.LinkTraceIDs, sr.LinkSpanIDs, sr.LinkTraceStates)
				values = w.layout.appendAttributes2D(values, sr.LinkAttributes)
				values = append(values, sr.ServiceName)
				values = w.layout.appendAttributes(values, sr.ResourceAttributes)
				values = append(values, sr.ScopeName, sr.ScopeVersion)
				values = w.layout.appendAttributes(values, sr.ScopeAttributes)
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
