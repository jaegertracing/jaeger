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
	logger := p.Logger
	if logger == nil {
		logger = zap.NewNop()
		p.Logger = logger
	}
	return &TraceWriter{
		spanWriter: spanstore.NewSpanWriter(p),
		logger:     logger,
	}
}

// WriteTraces convert the traces to ES Span model and write into the database
func (t *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	rs := td.ResourceSpans()
	if rs.Len() == 0 {
		t.logger.Debug("skipping write of empty trace data")
		return nil
	}

	dbSpans := ToDBModel(td)
	if len(dbSpans) == 0 {
		scopeSpansCount := 0
		spanCount := 0
		for i := 0; i < rs.Len(); i++ {
			scopeSpans := rs.At(i).ScopeSpans()
			scopeSpansCount += scopeSpans.Len()
			for j := 0; j < scopeSpans.Len(); j++ {
				spanCount += scopeSpans.At(j).Spans().Len()
			}
		}
		t.logger.Warn("no spans converted from trace data",
			zap.Int("resource_spans", rs.Len()),
			zap.Int("scope_spans", scopeSpansCount),
			zap.Int("spans", spanCount),
		)
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
