// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/pkg/es"
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
	ctx, span := s.spanReader.tracer.Start(ctx, "GetTrace")
	defer span.End()
	// TODO: use start time & end time in "query" struct
	traceIDs := make(map[dbmodel.TraceID]string)
	traceIDs[dbmodel.TraceID(query.TraceID.String())] = getLegacyTraceId(query.TraceID)
	traceMap, err := s.spanReader.GetTrace(ctx, traceIDs)
	if err != nil {
		return nil, es.DetailedError(err)
	}
	traces, err := s.toTraceModels(span, traceMap)
	if err != nil {
		return nil, es.DetailedError(err)
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return traces[0], nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReaderV1) GetServices(ctx context.Context) ([]string, error) {
	return s.spanReader.GetServices(ctx)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReaderV1) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	operations, err := s.spanReader.GetOperations(ctx, esOperationQueryParamsFromSpanStoreQueryParams(query))
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/jaegertracing/jaeger/issues/1923
	// 	- return the operations with actual span kind that meet requirement
	var result []spanstore.Operation
	for _, operation := range operations {
		result = append(result, spanstore.Operation{
			Name: operation.Name,
		})
	}
	return result, err
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReaderV1) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ctx, span := s.spanReader.tracer.Start(ctx, "FindTraces")
	defer span.End()

	uniqueTraceIDs, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, es.DetailedError(err)
	}
	return s.multiRead(ctx, uniqueTraceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReaderV1) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ctx, span := s.spanReader.tracer.Start(ctx, "FindTraceIDs")
	defer span.End()

	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}

	esTraceIDs, err := s.spanReader.FindTraceIDs(ctx, esTraceQueryParamsFromSpanStoreTraceQueryParams(traceQuery))
	if err != nil {
		return nil, err
	}

	return convertTraceIDsStringsToModels(esTraceIDs)
}

func (s *SpanReaderV1) multiRead(ctx context.Context, traceIDs []model.TraceID, startTime, endTime time.Time) ([]*model.Trace, error) {
	ctx, childSpan := s.tracer.Start(ctx, "multiRead")
	defer childSpan.End()
	traceIds := make(map[dbmodel.TraceID]string)
	for _, traceID := range traceIDs {
		traceIds[dbmodel.TraceID(traceID.String())] = getLegacyTraceId(traceID)
	}
	tracesMap, err := s.spanReader.MultiRead(ctx, traceIds, startTime, endTime)
	if err != nil {
		return []*model.Trace{}, err
	}
	return s.toTraceModels(childSpan, tracesMap)
}

func (s *SpanReaderV1) toTraceModels(childSpan trace.Span, tracesMap map[dbmodel.TraceID][]*dbmodel.Span) ([]*model.Trace, error) {
	var traces []*model.Trace
	for _, jsonSpans := range tracesMap {
		spans, err := s.collectSpans(jsonSpans)
		if err != nil {
			err = es.DetailedError(err)
			logErrorToSpan(childSpan, err)
			return nil, err
		}
		traces = append(traces, &model.Trace{Spans: spans})
	}
	return traces, nil
}

func getLegacyTraceId(traceID model.TraceID) string {
	// https://github.com/jaegertracing/jaeger/pull/1956 added leading zeros to IDs
	// So we need to also read IDs without leading zeros for compatibility with previously saved data.
	// TODO remove in newer versions, added in Jaeger 1.16
	if traceID.High == 0 {
		return strconv.FormatUint(traceID.Low, 16)
	}
	return fmt.Sprintf("%x%016x", traceID.High, traceID.Low)
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

func esOperationQueryParamsFromSpanStoreQueryParams(p spanstore.OperationQueryParameters) dbmodel.OperationQueryParameters {
	return dbmodel.OperationQueryParameters{
		ServiceName: p.ServiceName,
		SpanKind:    p.SpanKind,
	}
}

func esTraceQueryParamsFromSpanStoreTraceQueryParams(p *spanstore.TraceQueryParameters) *dbmodel.TraceQueryParameters {
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
