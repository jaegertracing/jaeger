// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type TraceWriter struct {
	spanWriter spanstore.Writer
}

func NewTraceWriter(spanWriter spanstore.Writer) *TraceWriter {
	return &TraceWriter{
		spanWriter: spanWriter,
	}
}

// WriteTraces implements tracestore.Writer.
func (t *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	batches := ProtoFromTraces(td)
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

type DependencyWriter struct {
	DependencyWriter dependencystore.Writer
}

func NewDependencyWriter(dependencyWriter dependencystore.Writer) *DependencyWriter {
	return &DependencyWriter{
		DependencyWriter: dependencyWriter,
	}
}

func (dw *DependencyWriter) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	return dw.DependencyWriter.WriteDependencies(ts, dependencies)
}
