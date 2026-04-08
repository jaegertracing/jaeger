// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
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
func (t *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	dbSpans := ToDBModel(td)
	if len(dbSpans) == 0 {
		return nil
	}

	spans := make([]*dbmodel.Span, len(dbSpans))
	startTimes := make([]time.Time, len(dbSpans))
	for i := range dbSpans {
		spans[i] = &dbSpans[i]
		startTimes[i] = model.EpochMicrosecondsAsTime(dbSpans[i].StartTime)
	}

	return t.spanWriter.WriteSpansSync(ctx, spans, startTimes)
}
func (t *TraceWriter) Close() error {
	return t.spanWriter.Close()
}
