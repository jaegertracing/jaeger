// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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

func (t *TraceReader) GetTraces(ctx context.Context, params ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		dbTraceIds := make([]dbmodel.TraceID, 0, len(params))
		for _, id := range params {
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

func (t *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	dbOperations, err := t.spanReader.GetOperations(ctx, dbmodel.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil {
		return nil, err
	}
	operations := make([]tracestore.Operation, 0, len(dbOperations))
	for _, op := range dbOperations {
		operations = append(operations, tracestore.Operation{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		})
	}
	return operations, nil
}

func (t *TraceReader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
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

func (t *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		traceIds, err := t.spanReader.FindTraceIDs(ctx, toDBTraceQueryParams(query))
		if err != nil {
			yield(nil, err)
			return
		}
		otelTraceIds := make([]tracestore.FoundTraceID, 0, len(traceIds))
		for _, traceId := range traceIds {
			dbTraceId, err := fromDbTraceId(traceId)
			if err != nil {
				yield(nil, err)
				return
			}
			otelTraceIds = append(otelTraceIds, tracestore.FoundTraceID{
				TraceID: dbTraceId,
			})
		}
		yield(otelTraceIds, nil)
	}
}

func toDBTraceQueryParams(query tracestore.TraceQueryParams) dbmodel.TraceQueryParameters {
	tags := make(map[string]string)
	for key, val := range query.Attributes.All() {
		tags[key] = val.AsString()
	}
	return dbmodel.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		Tags:          tags,
		NumTraces:     query.SearchDepth,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
	}
}
