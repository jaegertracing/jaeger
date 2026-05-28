// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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

// WriteTraces converts the traces to the ES span model and enqueues them into
// the bulk processor. The underlying bulk processor is asynchronous, so most
// write failures are reported via the processor's After callback rather than
// directly here. To honor the tracestore.Writer contract on a best-effort
// basis we drain any bulk-write error that was recorded since the previous
// call before enqueueing the current batch, returning it as the result of
// this call. Errors are therefore associated with the next subsequent call
// rather than the failing one; see issue #8476 for the design discussion.
func (t *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	prevErr := t.spanWriter.TakeBulkError()
	dbSpans := ToDBModel(td)
	for i := range dbSpans {
		span := &dbSpans[i]
		t.spanWriter.WriteSpan(model.EpochMicrosecondsAsTime(span.StartTime), span)
	}
	return prevErr
}

func (t *TraceWriter) Close() error {
	return t.spanWriter.Close()
}
