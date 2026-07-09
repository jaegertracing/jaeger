// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
)

type TraceWriter struct {
	spanWriter core.Writer
}

// NewTraceWriter returns the TraceWriter for use
func NewTraceWriter(p core.SpanWriterParams) *TraceWriter {
	return &TraceWriter{
		spanWriter: core.NewSpanWriter(p),
	}
}

// WriteTraces converts the traces to the ES span model and writes the batch to
// the database, returning any write error the core writer reports.
func (t *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	return t.spanWriter.WriteSpans(ctx, ToDBModel(td))
}

func (t *TraceWriter) Close() error {
	return t.spanWriter.Close()
}
