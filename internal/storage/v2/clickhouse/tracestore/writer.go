package tracestore

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
	batch, err := w.conn.PrepareBatch(ctx, sql.SpansInsert)
	if err != nil {
		return err
	}
	defer batch.Close()

	for _, rs := range td.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds()
				batch.Append(
					span.SpanID().String(),
					span.TraceID().String(),
					span.TraceState().AsRaw(),
					span.ParentSpanID().String(),
					span.Name(),
					span.Kind(),
					span.StartTimestamp().AsTime(),
					span.Status().Code(),
					span.Status().Message(),
					duration,
				)
			}
		}
	}

	return batch.Send()
}
