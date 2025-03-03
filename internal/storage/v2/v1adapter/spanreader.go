// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"errors"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ spanstore.Reader = (*SpanReader)(nil)

var errTooManyTracesFound = errors.New("too many traces found")

// SpanReader wraps a tracestore.Reader so that it can be downgraded to implement
// the v1 spanstore.Reader interface.
type SpanReader struct {
	traceReader tracestore.Reader
}

func NewSpanReader(traceReader tracestore.Reader) *SpanReader {
	return &SpanReader{
		traceReader: traceReader,
	}
}

func (sr *SpanReader) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	getTracesIter := sr.traceReader.GetTraces(ctx, tracestore.GetTraceParams{
		TraceID: FromV1TraceID(query.TraceID),
		Start:   query.StartTime,
		End:     query.EndTime,
	})
	traces, err := V1TracesFromSeq2(getTracesIter)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	} else if len(traces) > 1 {
		return nil, errTooManyTracesFound
	}
	return traces[0], nil
}

func (sr *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	return sr.traceReader.GetServices(ctx)
}

func (sr *SpanReader) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	o, err := sr.traceReader.GetOperations(ctx, tracestore.OperationQueryParams{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil || o == nil {
		return nil, err
	}
	operations := []spanstore.Operation{}
	for _, operation := range o {
		operations = append(operations, spanstore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	return operations, nil
}

func (sr *SpanReader) FindTraces(
	ctx context.Context,
	query *spanstore.TraceQueryParameters,
) ([]*model.Trace, error) {
	getTracesIter := sr.traceReader.FindTraces(ctx, tracestore.TraceQueryParams{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Attributes:    jptrace.PlainMapToPcommonMap(query.Tags),
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		SearchDepth:   query.NumTraces,
	})
	return V1TracesFromSeq2(getTracesIter)
}

func (sr *SpanReader) FindTraceIDs(
	ctx context.Context,
	query *spanstore.TraceQueryParameters,
) ([]model.TraceID, error) {
	traceIDsIter := sr.traceReader.FindTraceIDs(ctx, tracestore.TraceQueryParams{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Attributes:    jptrace.PlainMapToPcommonMap(query.Tags),
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		SearchDepth:   query.NumTraces,
	})
	return V1TraceIDsFromSeq2(traceIDsIter)
}
