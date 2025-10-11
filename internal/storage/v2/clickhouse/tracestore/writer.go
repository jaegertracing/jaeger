// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
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
		serviceName, _ := rs.Resource().Attributes().Get(otelsemconv.ServiceNameKey)
		resourceAttrs := dbmodel.ExtractAttributes(rs.Resource().Attributes())

		for _, ss := range rs.ScopeSpans().All() {
			scopeAttrs := dbmodel.ExtractAttributes(ss.Scope().Attributes())

			for _, span := range ss.Spans().All() {
				// Combine resource, scope, and span attributes
				spanAttrs := dbmodel.ExtractAttributes(span.Attributes())
				allAttrs := dbmodel.CombineAttributes(resourceAttrs, scopeAttrs, spanAttrs)

				// Extract events
				var eventNames []string
				var eventTimestamps []int64
				var eventBoolKeys, eventDoubleKeys, eventIntKeys, eventStrKeys, eventBytesKeys [][]string
				var eventBoolVals [][]bool
				var eventDoubleVals [][]float64
				var eventIntVals [][]int64
				var eventStrVals, eventBytesVals [][]string

				for _, event := range span.Events().All() {
					eventNames = append(eventNames, event.Name())
					eventTimestamps = append(eventTimestamps, event.Timestamp().AsTime().UnixNano())

					evtAttrs := dbmodel.ExtractAttributes(event.Attributes())
					eventBoolKeys = append(eventBoolKeys, evtAttrs.BoolKeys)
					eventBoolVals = append(eventBoolVals, evtAttrs.BoolValues)
					eventDoubleKeys = append(eventDoubleKeys, evtAttrs.DoubleKeys)
					eventDoubleVals = append(eventDoubleVals, evtAttrs.DoubleValues)
					eventIntKeys = append(eventIntKeys, evtAttrs.IntKeys)
					eventIntVals = append(eventIntVals, evtAttrs.IntValues)
					eventStrKeys = append(eventStrKeys, evtAttrs.StrKeys)
					eventStrVals = append(eventStrVals, evtAttrs.StrValues)
					eventBytesKeys = append(eventBytesKeys, evtAttrs.BytesKeys)
					eventBytesVals = append(eventBytesVals, evtAttrs.BytesValues)
				}

				// Extract links
				var linkTraceIDs, linkSpanIDs, linkTraceStates []string
				for _, link := range span.Links().All() {
					linkTraceIDs = append(linkTraceIDs, link.TraceID().String())
					linkSpanIDs = append(linkSpanIDs, link.SpanID().String())
					linkTraceStates = append(linkTraceStates, link.TraceState().AsRaw())
				}

				duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds()

				// Combine str and bytes attributes into "complex" for storage
				// ClickHouse schema has: bool, double, int, str, complex
				complexKeys := append(append([]string{}, allAttrs.StrKeys...), allAttrs.BytesKeys...)
				complexVals := append(append([]string{}, allAttrs.StrValues...), allAttrs.BytesValues...)

				err = batch.Append(
					span.SpanID().String(),
					span.TraceID().String(),
					span.TraceState().AsRaw(),
					span.ParentSpanID().String(),
					span.Name(),
					jptrace.SpanKindToString(span.Kind()),
					span.StartTimestamp().AsTime(),
					span.Status().Code().String(),
					span.Status().Message(),
					duration,
					allAttrs.BoolKeys,
					allAttrs.BoolValues,
					allAttrs.DoubleKeys,
					allAttrs.DoubleValues,
					allAttrs.IntKeys,
					allAttrs.IntValues,
					complexKeys,
					complexVals,
					eventNames,
					eventTimestamps,
					eventBoolKeys,
					eventBoolVals,
					eventDoubleKeys,
					eventDoubleVals,
					eventIntKeys,
					eventIntVals,
					eventStrKeys,
					eventStrVals,
					eventBytesKeys,
					eventBytesVals,
					linkTraceIDs,
					linkSpanIDs,
					linkTraceStates,
					serviceName.Str(),
					ss.Scope().Name(),
					ss.Scope().Version(),
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
