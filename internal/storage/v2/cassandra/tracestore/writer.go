// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/multierr"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
)

type TraceWriter struct {
	writer *spanstore.SpanWriter
}

func NewTraceWriter(writer *spanstore.SpanWriter) *TraceWriter {
	return &TraceWriter{writer: writer}
}

func (w *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	var errs error
	for _, ds := range ToDBModel(td) {
		if err := w.writer.WriteDBSpan(ctx, &ds); err != nil {
			errs = multierr.Append(errs, err)
		}
	}
	return errs
}
