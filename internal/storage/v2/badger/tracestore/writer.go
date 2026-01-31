// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

var _ tracestore.Writer = (*TraceWriter)(nil)

// TraceWriter handles writing traces to Badger storage using the v2 API.
type TraceWriter struct {
	spanWriter spanstore.Writer
}

// NewTraceWriter creates a new TraceWriter backed by Badger storage.
func NewTraceWriter(spanWriter spanstore.Writer) *TraceWriter {
	return &TraceWriter{
		spanWriter: spanWriter,
	}
}

// WriteTraces writes traces to Badger storage.
// This is a native v2 implementation that converts OTLP traces to v1 spans
// and writes them using the underlying v1 span writer.
func (w *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	// Convert OTLP traces to v1 batches
	batches := v1adapter.V1BatchesFromTraces(td)

	var errs []error
	for _, batch := range batches {
		for _, span := range batch.Spans {
			// Ensure process is set for each span
			if span.Process == nil {
				span.Process = batch.Process
			}

			// Write each span individually
			if err := w.spanWriter.WriteSpan(ctx, span); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}
