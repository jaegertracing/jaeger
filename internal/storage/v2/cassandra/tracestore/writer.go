// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/ptrace"

	cspanstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
)

type TraceWriter struct {
	writer cspanstore.CoreSpanWriter
}

func NewTraceWriter(writer cspanstore.CoreSpanWriter) *TraceWriter {
	return &TraceWriter{writer: writer}
}

func (w *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	dbSpans := ToDBModel(td)
	var errs []error
	for i := range dbSpans {
		// Stop processing early if the context is cancelled or times out
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}

		if err := w.writer.WriteDbSpan(ctx, &dbSpans[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
