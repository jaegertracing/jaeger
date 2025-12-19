// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	v1api "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

type TraceReader struct {
	reader spanstore.CoreSpanReader
}

func NewTraceReader(reader spanstore.CoreSpanReader) *TraceReader {
	return &TraceReader{reader: reader}
}

func (t *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return t.reader.GetServices(ctx)
}

func (t *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	operations, err := t.reader.GetOperations(ctx, query)
	if err != nil {
		return nil, err
	}
	result := make([]tracestore.Operation, len(operations))
	for i, operation := range operations {
		result[i] = tracestore.Operation{
			Name:     operation.OperationName,
			SpanKind: operation.SpanKind,
		}
	}
	return result, nil
}

func (t *TraceReader) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, idParams := range traceIDs {
			query := v1api.GetTraceParameters{
				TraceID:   v1adapter.ToV1TraceID(idParams.TraceID),
				StartTime: idParams.Start,
				EndTime:   idParams.End,
			}
			trace, err := t.reader.GetTrace(ctx, query)
			if err != nil {
				if errors.Is(err, v1api.ErrTraceNotFound) {
					continue
				}
				yield(nil, err)
				return
			}
			batch := &model.Batch{Spans: trace.GetSpans()}
			tr := v1adapter.V1BatchesToTraces([]*model.Batch{batch})
			if !yield([]ptrace.Traces{tr}, nil) {
				return
			}
		}
	}
}

func (t *TraceReader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traces, err := t.reader.FindTraces(ctx, query.ToSpanStoreQueryParameters())
		if err != nil {
			yield(nil, err)
			return
		}
		for _, trace := range traces {
			batch := &model.Batch{Spans: trace.GetSpans()}
			otelTrace := v1adapter.V1BatchesToTraces([]*model.Batch{batch})
			if !yield([]ptrace.Traces{otelTrace}, nil) {
				return
			}
		}
	}
}

func (t *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		traceIDs, err := t.reader.FindTraceIDs(ctx, query.ToSpanStoreQueryParameters())
		if err != nil {
			yield(nil, err)
			return
		}
		otelIDs := make([]tracestore.FoundTraceID, 0, len(traceIDs))
		for _, traceID := range traceIDs {
			otelIDs = append(otelIDs, tracestore.FoundTraceID{
				TraceID: v1adapter.FromV1TraceID(traceID),
			})
		}
		yield(otelIDs, nil)
	}
}
