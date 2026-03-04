// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
)

type TraceWriter struct {
	spanWriter spanstore.CoreSpanWriter
}

// NewTraceWriter returns the TraceWriter for use
func NewTraceWriter(p spanstore.SpanWriterParams) *TraceWriter {
	return &TraceWriter{
		spanWriter: spanstore.NewSpanWriter(p),
	}
}

// WriteTraces convert the traces to ES Span model and write into the database
func (t *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	dbSpans := ToDBModel(td)
	for i := range dbSpans {
		span := &dbSpans[i]
		t.spanWriter.WriteSpan(model.EpochMicrosecondsAsTime(span.StartTime), span)
	}
	return nil
}

func (t *TraceWriter) Close() error {
	return t.spanWriter.Close()
}
