// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
)

type TraceWriter struct {
	writer *spanstore.SpanWriter
}

func NewTraceWriter(writer *spanstore.SpanWriter) *TraceWriter {
	return &TraceWriter{
		writer: writer,
	}
}

func (w *TraceWriter) WriteTraces(ctx context.Context, traces ptrace.Traces) error {
	dbSpans := ToDBModel(traces)
	for i := range dbSpans {
		// Pass false for isFirehoseEnabled or rely on the flags directly in WriteDBSpan
		if err := w.writer.WriteDBSpan(ctx, &dbSpans[i]); err != nil {
			return err
		}
	}
	return nil
}
