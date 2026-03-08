// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type TraceReader struct {
	reader spanstore.CoreSpanReader
}

func NewTraceReader(session cassandra.Session,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	tracer trace.Tracer,
) (*TraceReader, error) {
	coreReader, err := spanstore.NewSpanReader(session, metricsFactory, logger, tracer)
	if err != nil {
		return nil, err
	}
	return &TraceReader{reader: coreReader}, nil
}

func (r *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return r.reader.GetServices(ctx)
}

func (r *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return r.reader.GetOperationsV2(ctx, query)
}

func (r *TraceReader) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, traceID := range traceIDs {
			dbTrace, err := r.reader.GetTrace(ctx, traceID)
			if err != nil {
				yield(nil, err)
				return
			}
			td := FromDBModel(dbTrace.Spans)
			if !yield([]ptrace.Traces{td}, nil) {
				return
			}
		}
	}
}

func (r *TraceReader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		dbTraces, err := r.reader.FindTraces(ctx, &query)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, dbTrace := range dbTraces {
			td := FromDBModel(dbTrace.Spans)
			if !yield([]ptrace.Traces{td}, nil) {
				return
			}
		}
	}
}

func (r *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		dbTraceIDs, err := r.reader.FindTraceIDs(ctx, &query)
		if err != nil {
			yield(nil, err)
			return
		}
		otelTraceIDs := make([]tracestore.FoundTraceID, 0, len(dbTraceIDs))
		for _, dbTraceID := range dbTraceIDs {
			otelTraceIDs = append(otelTraceIDs, tracestore.FoundTraceID{
				TraceID: pcommon.TraceID(dbTraceID),
			})
		}
		yield(otelTraceIDs, nil)
	}
}
