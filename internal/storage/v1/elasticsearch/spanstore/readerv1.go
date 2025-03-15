// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
)

var _ spanstore.Reader = (*SpanReaderV1)(nil) // check API conformance

// SpanReaderV1	is a wrapper around SpanReader
type SpanReaderV1 struct {
	spanReader    CoreSpanReader
	spanConverter dbmodel.ToDomain
}

// NewSpanReaderV1 returns an instance of SpanReaderV1
func NewSpanReaderV1(p SpanReaderParams) *SpanReaderV1 {
	return &SpanReaderV1{
		spanReader:    NewSpanReader(p),
		spanConverter: dbmodel.NewToDomain(p.TagDotReplacement),
	}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReaderV1) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	traces, err := s.spanReader.GetTrace(ctx, []dbmodel.TraceID{dbmodel.TraceID(query.TraceID.String())})
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	spans, err := s.collectSpans(traces[0].Spans)
	if err != nil {
		return nil, err
	}
	return &model.Trace{Spans: spans}, nil
}

func (s *SpanReaderV1) collectSpans(jsonSpans []*dbmodel.Span) ([]*model.Span, error) {
	spans := make([]*model.Span, len(jsonSpans))
	for i, jsonSpan := range jsonSpans {
		span, err := s.spanConverter.SpanToDomain(jsonSpan)
		if err != nil {
			return nil, fmt.Errorf("converting JSONSpan to domain Span failed: %w", err)
		}
		spans[i] = span
	}
	return spans, nil
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReaderV1) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	dbmodelQuery := dbmodel.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	}
	operations, err := s.spanReader.GetOperations(ctx, dbmodelQuery)
	if err != nil {
		return nil, err
	}
	var result []spanstore.Operation

	for _, operation := range operations {
		result = append(result, spanstore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
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
	traces, err := s.spanReader.FindTraces(ctx, toDbQueryParams(traceQuery))
	if err != nil {
		return nil, err
	}
	var result []*model.Trace
	for _, trace := range traces {
		spans, err := s.collectSpans(trace.Spans)
		if err != nil {
			return nil, err
		}
		result = append(result, &model.Trace{Spans: spans})
	}
	return result, nil
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReaderV1) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ids, err := s.spanReader.FindTraceIDs(ctx, toDbQueryParams(traceQuery))
	if err != nil {
		return nil, err
	}
	return toModelTraceIDs(ids)
}

func toDbQueryParams(p *spanstore.TraceQueryParameters) *dbmodel.TraceQueryParameters {
	return &dbmodel.TraceQueryParameters{
		ServiceName:   p.ServiceName,
		OperationName: p.OperationName,
		Tags:          p.Tags,
		StartTimeMin:  p.StartTimeMin,
		StartTimeMax:  p.StartTimeMax,
		DurationMin:   p.DurationMin,
		DurationMax:   p.DurationMax,
		NumTraces:     p.NumTraces,
	}
}

func toModelTraceIDs(traceIDs []dbmodel.TraceID) ([]model.TraceID, error) {
	traceIDsMap := map[model.TraceID]bool{}
	// https://github.com/jaegertracing/jaeger/pull/1956 added leading zeros to IDs
	// So we need to also read IDs without leading zeros for compatibility with previously saved data.
	// That means the input to this function may contain logically identical trace IDs but formatted
	// with or without padding, and we need to dedupe them.
	// TODO remove deduping in newer versions, added in Jaeger 1.16
	traceIDsModels := make([]model.TraceID, 0, len(traceIDs))
	for _, ID := range traceIDs {
		traceID, err := model.TraceIDFromString(string(ID))
		if err != nil {
			return nil, fmt.Errorf("making traceID from string '%s' failed: %w", ID, err)
		}
		if _, ok := traceIDsMap[traceID]; !ok {
			traceIDsMap[traceID] = true
			traceIDsModels = append(traceIDsModels, traceID)
		}
	}

	return traceIDsModels, nil
}
