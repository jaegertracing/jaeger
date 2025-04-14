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

type TraceReader struct {
	spanReader spanstore.CoreSpanReader
}

func NewTraceReader(p spanstore.SpanReaderParams) *TraceReader {
	return &TraceReader{
		spanReader: spanstore.NewSpanReader(p),
	}
}

func (t *TraceReader) GetTraces(ctx context.Context, traceIDs ...v2api.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		dbTraceIds := make([]dbmodel.TraceID, 0, len(traceIDs))
		for _, id := range traceIDs {
			dbTraceIds = append(dbTraceIds, dbmodel.TraceID(id.TraceID.String()))
		}
		dbTraces, err := t.spanReader.GetTraces(ctx, dbTraceIds)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, trace := range dbTraces {
			td, err := FromDBModel(trace.Spans)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield([]ptrace.Traces{td}, nil) {
				return
			}
		}
	}
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

func (t *TraceReader) FindTraces(ctx context.Context, query v2api.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traces, err := t.spanReader.FindTraces(ctx, toDBTraceQueryParams(query))
		if err != nil {
			yield(nil, err)
			return
		}
		for _, trace := range traces {
			td, err := FromDBModel(trace.Spans)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield([]ptrace.Traces{td}, nil) {
				return
			}
		}
	}
}

func (t *TraceReader) FindTraceIDs(ctx context.Context, query v2api.TraceQueryParams) iter.Seq2[[]v2api.FoundTraceID, error] {
	return func(yield func([]v2api.FoundTraceID, error) bool) {
		traceIds, err := t.spanReader.FindTraceIDs(ctx, toDBTraceQueryParams(query))
		if err != nil {
			yield(nil, err)
			return
		}
		otelTraceIds := make([]v2api.FoundTraceID, 0, len(traceIds))
		for _, dbTraceId := range traceIds {
			traceId, err := fromDbTraceId(dbTraceId)
			if err != nil {
				yield(nil, err)
				return
			}
			otelTraceIds = append(otelTraceIds, v2api.FoundTraceID{
				TraceID: traceId,
			})
		}
		yield(otelTraceIds, nil)
	}
}

func toDBTraceQueryParams(query v2api.TraceQueryParams) dbmodel.TraceQueryParameters {
	rawMap := make(map[string]string)
	for key, val := range query.Attributes.All() {
		rawMap[key] = val.AsString()
	}
	return dbmodel.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     query.SearchDepth,
		Tags:          rawMap,
	}
}
