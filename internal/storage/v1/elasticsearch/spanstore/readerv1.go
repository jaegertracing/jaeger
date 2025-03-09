// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
)

// SpanReaderV1	is a wrapper around SpanReader
type SpanReaderV1 struct {
	spanReader    *SpanReader
	spanConverter dbmodel.ToDomain
	tracer        trace.Tracer
}

// NewSpanReaderV1 returns an instance of SpanReaderV1
func NewSpanReaderV1(p SpanReaderParams) *SpanReaderV1 {
	return &SpanReaderV1{
		spanReader:    NewSpanReader(p),
		spanConverter: dbmodel.NewToDomain(p.TagDotReplacement),
		tracer:        p.Tracer,
	}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReaderV1) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	return s.spanReader.GetTrace(ctx, query)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReaderV1) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	dbmodelQuery := dbmodel.OperationQueryParameters{
		ServiceName: query.ServiceName,
	}
	operations, err := s.spanReader.GetOperations(ctx, dbmodelQuery)
	if err != nil {
		return nil, err
	}
	var result []spanstore.Operation

	for _, operation := range operations {
		result = append(result, spanstore.Operation{
			Name: operation.Name,
		})
	}
	return result, nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReaderV1) GetServices(ctx context.Context) ([]string, error) {
	return s.spanReader.GetServices(ctx)
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReaderV1) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return s.spanReader.FindTraces(ctx, traceQuery)
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReaderV1) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return s.spanReader.FindTraceIDs(ctx, traceQuery)
}
