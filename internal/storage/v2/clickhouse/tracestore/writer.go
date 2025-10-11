// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
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
				sr := spanToRow(rs.Resource(), ss.Scope(), span)
				err = batch.Append(
					sr.id,
					sr.traceID,
					sr.traceState,
					sr.parentSpanID,
					sr.name,
					sr.kind,
					sr.startTime,
					sr.statusCode,
					sr.statusMessage,
					sr.rawDuration,
					sr.serviceName,
					sr.scopeName,
					sr.scopeVersion,
					sr.boolAttributeKeys,
					sr.boolAttributeValues,
					sr.doubleAttributeKeys,
					sr.doubleAttributeValues,
					sr.intAttributeKeys,
					sr.intAttributeValues,
					sr.strAttributeKeys,
					sr.strAttributeValues,
					sr.complexAttributeKeys,
					sr.complexAttributeValues,
					sr.eventNames,
					sr.eventTimestamps,
					sr.eventBoolAttributeKeys,
					sr.eventBoolAttributeValues,
					sr.eventDoubleAttributeKeys,
					sr.eventDoubleAttributeValues,
					sr.eventIntAttributeKeys,
					sr.eventIntAttributeValues,
					sr.eventStrAttributeKeys,
					sr.eventStrAttributeValues,
					sr.eventComplexAttributeKeys,
					sr.eventComplexAttributeValues,
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
