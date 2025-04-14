// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	v2api "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TraceReader is a wrapper around spanstore.CoreSpanReader which return the output parallel to OTLP Models
type TraceReader struct {
	spanReader spanstore.CoreSpanReader
}

// NewTraceReader returns an instance of TraceReader
func NewTraceReader(p spanstore.SpanReaderParams) *TraceReader {
	return &TraceReader{
		spanReader: spanstore.NewSpanReader(p),
	}
}

func (*TraceReader) GetTraces(_ context.Context, _ ...v2api.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	panic("not implemented")
}

func (t *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return t.spanReader.GetServices(ctx)
}

func (t *TraceReader) GetOperations(ctx context.Context, query v2api.OperationQueryParams) ([]v2api.Operation, error) {
	dbOperations, err := t.spanReader.GetOperations(ctx, dbmodel.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil {
		return nil, err
	}
	operations := make([]v2api.Operation, 0, len(dbOperations))
	for _, op := range dbOperations {
		operations = append(operations, v2api.Operation{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		})
	}
	return operations, nil
}

func (*TraceReader) FindTraces(_ context.Context, _ v2api.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	panic("not implemented")
}

func (*TraceReader) FindTraceIDs(_ context.Context, _ v2api.TraceQueryParams) iter.Seq2[[]v2api.FoundTraceID, error] {
	panic("not implemented")
}
