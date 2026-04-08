// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ spanstore.Reader = (*SpanReaderV1)(nil)

// SpanReaderV1 wraps CoreSpanReader and adapts it to the v1 spanstore.Reader interface,
// converting Cassandra dbmodel types to the legacy Jaeger domain model.
type SpanReaderV1 struct {
	reader CoreSpanReader
}

// NewSpanReaderV1 returns a new SpanReaderV1 wrapping the given CoreSpanReader.
func NewSpanReaderV1(reader CoreSpanReader) *SpanReaderV1 {
	return &SpanReaderV1{reader: reader}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReaderV1) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	spans, err := s.reader.GetTrace(ctx, dbmodel.TraceIDFromDomain(query.TraceID))
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	domainSpans := make([]*model.Span, 0, len(spans))
	for i := range spans {
		span, err := dbmodel.ToDomain(&spans[i])
		if err != nil {
			return nil, err
		}
		domainSpans = append(domainSpans, span)
	}
	return &model.Trace{Spans: domainSpans}, nil
}

// GetServices returns all services traced by Jaeger
func (s *SpanReaderV1) GetServices(ctx context.Context) ([]string, error) {
	return s.reader.GetServices(ctx)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReaderV1) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return s.reader.GetOperations(ctx, query)
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReaderV1) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	dbTraces, err := s.reader.FindTraces(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	traces := make([]*model.Trace, 0, len(dbTraces))
	for _, dbTrace := range dbTraces {
		domainSpans := make([]*model.Span, 0, len(dbTrace.Spans))
		for i := range dbTrace.Spans {
			span, err := dbmodel.ToDomain(&dbTrace.Spans[i])
			if err != nil {
				return nil, err
			}
			domainSpans = append(domainSpans, span)
		}
		traces = append(traces, &model.Trace{Spans: domainSpans})
	}
	return traces, nil
}

// FindTraceIDs retrieves traceIDs that match the traceQuery
func (s *SpanReaderV1) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	dbIDs, err := s.reader.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	ids := make([]model.TraceID, 0, len(dbIDs))
	for _, id := range dbIDs {
		ids = append(ids, id.ToDomain())
	}
	return ids, nil
}
