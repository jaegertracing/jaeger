// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	cassdbmodel "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type TraceReader struct {
	reader spanstore.CoreSpanReader
}

func NewTraceReader(reader spanstore.CoreSpanReader) *TraceReader {
	return &TraceReader{reader: reader}
}

func (r *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return r.reader.GetServices(ctx)
}

func (r *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return r.reader.GetOperations(ctx, query)
}

func (r *TraceReader) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, id := range traceIDs {
			// pcommon.TraceID and cassdbmodel.TraceID are both [16]byte with same byte layout
			spans, err := r.reader.GetTrace(ctx, cassdbmodel.TraceID(id.TraceID))
			if err != nil {
				yield(nil, err)
				return
			}
			if len(spans) == 0 {
				continue
			}
			td := FromDBModel(spans)
			if !yield([]ptrace.Traces{td}, nil) {
				return
			}
		}
	}
}

func (r *TraceReader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for trace, err := range r.reader.FindTraces(ctx, &query) {
			if err != nil {
				yield(nil, err)
				return
			}
			td := FromDBModel(trace.Spans)
			if !yield([]ptrace.Traces{td}, nil) {
				return
			}
		}
	}
}

func (r *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		dbIDs, err := r.reader.FindTraceIDs(ctx, &query)
		if err != nil {
			yield(nil, err)
			return
		}
		if len(dbIDs) == 0 {
			return
		}
		otelIDs := make([]tracestore.FoundTraceID, 0, len(dbIDs))
		for _, id := range dbIDs {
			otelIDs = append(otelIDs, tracestore.FoundTraceID{
				TraceID: pcommon.TraceID(id),
			})
		}
		yield(otelIDs, nil)
	}
}
