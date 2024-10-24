// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/ptrace"

	spanstore_v1 "github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type TraceWriter struct {
	spanWriter spanstore_v1.Writer
}

func NewTraceWriter(spanWriter spanstore_v1.Writer) spanstore.Writer {
	return &TraceWriter{
		spanWriter: spanWriter,
	}
}

// WriteTraces implements spanstore.Writer.
func (t *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	batches := otlp2jaeger.ProtoFromTraces(td)
	var errs []error
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			err := t.spanWriter.WriteSpan(ctx, span)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
