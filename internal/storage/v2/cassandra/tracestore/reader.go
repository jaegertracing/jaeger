// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

type TraceReader struct {
	reader   spanstore.CoreSpanReader
	fallback tracestore.Reader
}

func NewTraceReader(reader spanstore.CoreSpanReader) *TraceReader {
	return &TraceReader{
		reader:   reader,
		fallback: v1adapter.NewTraceReader(reader),
	}
}

func (r *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return r.reader.GetServices(ctx)
}

func (r *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return r.reader.GetOperationsV2(ctx, query)
}

func (r *TraceReader) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return r.fallback.GetTraces(ctx, traceIDs...)
}

func (r *TraceReader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return r.fallback.FindTraces(ctx, query)
}

func (r *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return r.fallback.FindTraceIDs(ctx, query)
}
