// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"

	model2otel "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

var errV1ReaderNotAvailable = errors.New("spanstore.Reader is not a wrapper around v1 reader")

type TraceReader struct {
	spanReader spanstore.Reader
}

func GetV1Reader(reader tracestore.Reader) (spanstore.Reader, error) {
	if tr, ok := reader.(*TraceReader); ok {
		return tr.spanReader, nil
	}
	return nil, errV1ReaderNotAvailable
}

func NewTraceReader(spanReader spanstore.Reader) *TraceReader {
	return &TraceReader{
		spanReader: spanReader,
	}
}

func (tr *TraceReader) GetTrace(ctx context.Context, traceID pcommon.TraceID) (ptrace.Traces, error) {
	t, err := tr.spanReader.GetTrace(ctx, model.TraceIDFromOTEL(traceID))
	if err != nil {
		return ptrace.NewTraces(), err
	}
	batch := &model.Batch{Spans: t.GetSpans()}
	return model2otel.ProtoToTraces([]*model.Batch{batch})
}

func (tr *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return tr.spanReader.GetServices(ctx)
}

func (tr *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParameters) ([]tracestore.Operation, error) {
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
	query tracestore.TraceQueryParameters,
) ([]ptrace.Traces, error) {
	t, err := tr.spanReader.FindTraces(ctx, query.ToSpanStoreQueryParameters())
	if err != nil || t == nil {
		return nil, err
	}
	otelTraces := []ptrace.Traces{}
	for _, trace := range t {
		batch := &model.Batch{Spans: trace.GetSpans()}
		otelTrace, _ := model2otel.ProtoToTraces([]*model.Batch{batch})
		otelTraces = append(otelTraces, otelTrace)
	}
	return otelTraces, nil
}

func (tr *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParameters) ([]pcommon.TraceID, error) {
	t, err := tr.spanReader.FindTraceIDs(ctx, query.ToSpanStoreQueryParameters())
	if err != nil || t == nil {
		return nil, err
	}
	traceIDs := []pcommon.TraceID{}
	for _, traceID := range t {
		traceIDs = append(traceIDs, traceID.ToOTELTraceID())
	}
	return traceIDs, nil
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
