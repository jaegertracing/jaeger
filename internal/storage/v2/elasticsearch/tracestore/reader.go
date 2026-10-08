// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

var _ tracestore.Reader = (*TraceReader)(nil)

// TraceReader is a wrapper around core.Reader which returns the output parallel to OTLP Models
type TraceReader struct {
	spanReader core.Reader
}

// NewTraceReader returns an instance of TraceReader.
func NewTraceReader(p core.SpanReaderParams) *TraceReader {
	return &TraceReader{
		spanReader: core.NewSpanReader(p),
	}
}

func (*TraceReader) SearchCapabilities(context.Context) (tracestore.SearchCapabilities, error) {
	filter := core.FilterCapabilities()
	return tracestore.SearchCapabilities{
		// The query adds its process.serviceName clause only when the query carries a
		// name, and no other clause depends on it, so an omitted name matches spans from
		// every service.
		WithoutServiceName: true,
		// A search matches span documents, and the clauses of a conjunction all apply to the
		// same document, so a conjunction is satisfied within one span rather than across a
		// trace.
		SameSpanConjunction: true,
		Filter:              &filter,
		// FindSpans reads span documents in the order the caller selects, lowering every
		// term of the ordering contract to a single-valued document field (RFC 0016 §6).
		SpanSearch:  true,
		SpanSorting: true,
		// FindSpans pages with a keyset cursor over the engine's own sort values. The
		// capability covers the trace searches as well, which do not page yet: they serve
		// one page bounded by the page size and refuse a token (see paginationAsDepth).
		Paginated: true,
	}, nil
}

func (r *TraceReader) GetTraces(ctx context.Context, params ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		dbTraceIds := make([]dbmodel.TraceID, 0, len(params))
		for _, id := range params {
			dbTraceIds = append(dbTraceIds, dbmodel.TraceID(id.TraceID.String()))
		}
		dbTraces, err := r.spanReader.GetTraces(ctx, dbTraceIds)
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

func (r *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return r.spanReader.GetServices(ctx)
}

func (r *TraceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	dbOperations, err := r.spanReader.GetOperations(ctx, dbmodel.OperationQueryParameters{
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

func (r *TraceReader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traces, err := r.spanReader.FindTraces(ctx, toDBTraceQueryParams(query))
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

// FindSpans returns one page of the spans matching query, in the effective order, with the
// token that resumes the search (RFC 0016 §6). The core reader owns the cursor inside the
// token; this layer binds it to the query's fingerprint so that a token returned for one query
// is refused by every other (RFC 0014 §3.2).
func (r *TraceReader) FindSpans(ctx context.Context, query tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		// The query service settles the order before calling a reader, but a reader reached
		// directly must still refuse terms it cannot execute, and the settled order is the one
		// the fingerprint and the sort must agree on.
		order, err := tracestore.EffectiveSpanOrder(query.OrderBy)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		query.OrderBy = order
		fingerprint, err := query.Fingerprint()
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		var cursor []byte
		if query.Pagination.PageToken != "" {
			if cursor, err = query.Pagination.PageToken.Cursor(fingerprint); err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
		}
		// The query service clamps the page size (RFC 0014 §4); a caller reaching the reader
		// directly gets the same bound, and the conversion happens only under it.
		pageSize := int(tracestore.MaxPageSize)
		if query.Pagination.PageSize <= tracestore.MaxPageSize {
			pageSize = int(query.Pagination.PageSize)
		}
		page, err := r.spanReader.FindSpans(ctx, dbmodel.SpanQueryParameters{
			StartTimeMin: query.StartTimeMin,
			StartTimeMax: query.StartTimeMax,
			Filter:       query.Filter,
			OrderBy:      order,
			PageSize:     pageSize,
			Cursor:       cursor,
		})
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		// FromDBModel gives every span its own ResourceSpans, so the page keeps the engine's
		// order even where consecutive spans alternate between services (RFC 0016 §6.4).
		td, err := FromDBModel(page.Spans)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		chunk := tracestore.PageChunk[ptrace.Traces]{Results: td}
		if len(page.NextCursor) > 0 {
			if chunk.NextPageToken, err = tracestore.NewPageToken(fingerprint, page.NextCursor); err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
		}
		yield(chunk, nil)
	}
}

func (r *TraceReader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[tracestore.PageChunk[[]tracestore.FoundTraceID], error] {
	return func(yield func(tracestore.PageChunk[[]tracestore.FoundTraceID], error) bool) {
		query, err := paginationAsDepth(query)
		if err != nil {
			yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{}, err)
			return
		}
		traceIds, err := r.spanReader.FindTraceIDs(ctx, toDBTraceQueryParams(query))
		if err != nil {
			yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{}, err)
			return
		}
		otelTraceIds := make([]tracestore.FoundTraceID, 0, len(traceIds))
		for _, traceId := range traceIds {
			dbTraceId, err := traceId.ToOTEL()
			if err != nil {
				yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{}, err)
				return
			}
			otelTraceIds = append(otelTraceIds, tracestore.FoundTraceID{
				TraceID: dbTraceId,
			})
		}
		// The trace searches do not page yet (RFC 0014 M3), so the page is the last one.
		yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{Results: otelTraceIds}, nil)
	}
}

// paginationAsDepth is RFC 0014 §6.2 applied inside this reader for the trace searches, which
// do not page yet while the Paginated capability, declared for FindSpans, covers them too. The
// page size bounds the search as its depth, and a token is refused, because these searches
// cannot have produced one. The query service applies the same rule, in queryToReaderCapabilities,
// to a reader that declares no pagination at all.
func paginationAsDepth(query tracestore.TraceQueryParams) (tracestore.TraceQueryParams, error) {
	if query.Pagination == nil {
		return query, nil
	}
	if query.Pagination.PageToken != "" {
		return tracestore.TraceQueryParams{}, fmt.Errorf("%w: trace searches on this storage backend do not page yet", tracestore.ErrPaginationUnsupported)
	}
	query.SearchDepth = query.Pagination.PageSize
	query.Pagination = nil
	return query, nil
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
		Filter:        query.Filter,
	}
}
