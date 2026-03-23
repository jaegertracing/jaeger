// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
)

type TraceWriter struct {
	spanWriter spanstore.CoreSpanWriter
	logger     *zap.Logger
}

// NewTraceWriter returns the TraceWriter for use
func NewTraceWriter(p spanstore.SpanWriterParams) *TraceWriter {
	return &TraceWriter{
		spanWriter: spanstore.NewSpanWriter(p),
		logger:     p.Logger,
	}
}

// WriteTraces convert the traces to ES Span model and write into the database
func (t *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	dbSpans := ToDBModel(td)
	if len(dbSpans) == 0 {
		if td.ResourceSpans().Len() > 0 {
			t.logger.Warn("span conversion produced no spans from non-empty trace data",
				zap.Int("resource_spans", td.ResourceSpans().Len()),
			)
		} else {
			t.logger.Debug("skipping write of empty trace data")
		}
		return nil
	}
	for i := range dbSpans {
		span := &dbSpans[i]
		t.spanWriter.WriteSpan(model.EpochMicrosecondsAsTime(span.StartTime), span)
	}
	t.logger.Debug("wrote spans to ES", zap.Int("count", len(dbSpans)))
	return nil
}

func (t *TraceWriter) Close() error {
	return t.spanWriter.Close()
}
