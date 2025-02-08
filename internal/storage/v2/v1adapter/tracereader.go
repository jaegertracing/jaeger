// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/pkg/iter"
)

var _ tracestore.Reader = (*TraceReader)(nil)

type TraceReader struct {
	spanReader spanstore.Reader
}

func GetV1Reader(reader tracestore.Reader) (spanstore.Reader, bool) {
	if tr, ok := reader.(*TraceReader); ok {
		return tr.spanReader, ok
	}
	return nil, false
}

func NewTraceReader(spanReader spanstore.Reader) *TraceReader {
	return &TraceReader{
		spanReader: spanReader,
	}
}

func (tr *TraceReader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, idParams := range traceIDs {
			query := spanstore.GetTraceParameters{
				TraceID:   ToV1TraceID(idParams.TraceID),
				StartTime: idParams.Start,
				EndTime:   idParams.End,
			}
			t, err := tr.spanReader.GetTrace(ctx, query)
			if err != nil {
				if errors.Is(err, spanstore.ErrTraceNotFound) {
					continue
				}
				yield(nil, err)
				return
			}
			batch := &model.Batch{Spans: t.GetSpans()}
			tr := V1BatchesToTraces([]*model.Batch{batch})
			if !yield([]ptrace.Traces{tr}, nil) {
				return
			}
		}
	}
}

func (tr *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return tr.spanReader.GetServices(ctx)
}

func (tr *TraceReader) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	o, err := tr.spanReader.GetOperations(ctx, spanstore.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil || o == nil {
		return nil, err
	}
	operations := []tracestore.Operation{}
	for _, operation := range o {
		operations = append(operations, tracestore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	return operations, nil
}

func (tr *TraceReader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traces, err := tr.spanReader.FindTraces(ctx, query.ToSpanStoreQueryParameters())
		if err != nil {
			yield(nil, err)
			return
		}
		for _, trace := range traces {
			batch := &model.Batch{Spans: trace.GetSpans()}
			otelTrace := V1BatchesToTraces([]*model.Batch{batch})
			if !yield([]ptrace.Traces{otelTrace}, nil) {
				return
			}
		}
	}
}

func (tr *TraceReader) FindTraceIDs(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]pcommon.TraceID, error] {
	return func(yield func([]pcommon.TraceID, error) bool) {
		traceIDs, err := tr.spanReader.FindTraceIDs(ctx, query.ToSpanStoreQueryParameters())
		if err != nil {
			yield(nil, err)
			return
		}
		otelIDs := make([]pcommon.TraceID, 0, len(traceIDs))
		for _, traceID := range traceIDs {
			otelIDs = append(otelIDs, FromV1TraceID(traceID))
		}
		yield(otelIDs, nil)
	}
}

type DependencyReader struct {
	reader dependencystore.Reader
}

func NewDependencyReader(reader dependencystore.Reader) *DependencyReader {
	return &DependencyReader{
		reader: reader,
	}
}

func (dr *DependencyReader) GetDependencies(
	ctx context.Context,
	query depstore.QueryParameters,
) ([]model.DependencyLink, error) {
	return dr.reader.GetDependencies(ctx, query.EndTime, query.EndTime.Sub(query.StartTime))
}
