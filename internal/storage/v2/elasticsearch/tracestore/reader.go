// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

var (
	_ tracestore.Reader        = (*TraceReader)(nil)
	_ tracestore.SummaryReader = (*TraceReader)(nil)
)

// summaryAggregator is an optional capability kept out of core.Reader so existing
// implementations and their generated mocks are unaffected; the wrapper discovers
// it via a type assertion.
type summaryAggregator interface {
	FindTraceSummaries(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) ([]dbmodel.TraceSummary, error)
}

// TraceReader is a wrapper around core.Reader which returns the output parallel to OTLP Models
type TraceReader struct {
	spanReader core.Reader
}

// NewTraceReader returns an instance of TraceReader
func NewTraceReader(p core.SpanReaderParams) *TraceReader {
	return &TraceReader{
		spanReader: core.NewSpanReader(p),
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
			dbTraceId, err := convertTraceIDFromDB(traceId)
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

// FindTraceSummaries implements tracestore.SummaryReader. It yields
// errors.ErrUnsupported when the core reader has no native summary support, so the
// query service falls back to client-side aggregation.
func (t *TraceReader) FindTraceSummaries(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		aggregator, ok := t.spanReader.(summaryAggregator)
		if !ok {
			yield(nil, fmt.Errorf("native trace summaries are not supported by this reader: %w", errors.ErrUnsupported))
			return
		}

		dbSummaries, err := aggregator.FindTraceSummaries(ctx, toDBTraceQueryParams(query))
		if err != nil {
			yield(nil, err)
			return
		}

		summaries := make([]tracestore.TraceSummary, 0, len(dbSummaries))
		for _, dbSummary := range dbSummaries {
			summary, err := convertTraceSummaryFromDB(dbSummary)
			if err != nil {
				yield(nil, err)
				return
			}
			summaries = append(summaries, summary)
		}
		yield(summaries, nil)
	}
}

func convertTraceSummaryFromDB(dbSummary dbmodel.TraceSummary) (tracestore.TraceSummary, error) {
	traceID, err := convertTraceIDFromDB(dbSummary.TraceID)
	if err != nil {
		return tracestore.TraceSummary{}, err
	}

	services := make([]tracestore.ServiceSummary, 0, len(dbSummary.Services))
	for _, svc := range dbSummary.Services {
		services = append(services, tracestore.ServiceSummary{
			Name:           svc.ServiceName,
			SpanCount:      svc.SpanCount,
			ErrorSpanCount: svc.ErrorSpanCount,
		})
	}

	var minStart, maxEnd time.Time
	if dbSummary.MinStartTime > 0 {
		//nolint:gosec // G115: microsecond epoch timestamp is well within int64 range
		minStart = time.UnixMicro(int64(dbSummary.MinStartTime)).UTC()
	}
	if dbSummary.MaxEndTime > 0 {
		//nolint:gosec // G115: microsecond epoch timestamp is well within int64 range
		maxEnd = time.UnixMicro(int64(dbSummary.MaxEndTime)).UTC()
	}

	return tracestore.TraceSummary{
		TraceID:           traceID,
		RootServiceName:   dbSummary.RootServiceName,
		RootOperationName: dbSummary.RootOperationName,
		MinStartTime:      minStart,
		MaxEndTime:        maxEnd,
		SpanCount:         dbSummary.SpanCount,
		ErrorSpanCount:    dbSummary.ErrorSpanCount,
		// OrphanSpanCount stays zero on the native path; see FindTraceSummaries.
		OrphanSpanCount: 0,
		Services:        services,
	}, nil
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
		SearchDepth:   query.SearchDepth,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
	}
}
