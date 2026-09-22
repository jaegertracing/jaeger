// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// ErrInterceptorFilter reports that a query interceptor returned a filter jaeger-query will not
// send to storage. It is deliberately not one of the errors the API layers answer 400 for: the
// caller's request was fine, and the fault is in the extension this deployment configured.
var ErrInterceptorFilter = errors.New("query interceptor returned an invalid filter")

// toInterceptorTraceQuery and fromInterceptorTraceQuery convert at the contract boundary, so the
// internal query type never crosses it.
func toInterceptorTraceQuery(q tracestore.TraceQueryParams) queryinterceptor.TraceQuery {
	return queryinterceptor.TraceQuery{
		Filter:       q.Filter,
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
	}
}

func fromInterceptorTraceQuery(q queryinterceptor.TraceQuery, original tracestore.TraceQueryParams) tracestore.TraceQueryParams {
	return tracestore.TraceQueryParams{
		Attributes:   pcommon.NewMap(),
		Filter:       q.Filter,
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
		SearchDepth:  original.SearchDepth,
		Pagination:   original.Pagination,
	}
}

// onTraceQuery runs every interceptor's OnTraceQuery in order, threading the context each returns into the
// next. The final context is returned so the caller can pass it to the storage reader and to
// OnTraceResult, letting an interceptor carry per-query state (a resolved caller identity, say) from
// the pre-query hook to the return path.
//
// The interceptors are shown the query in filter shape whatever shape it arrived in, because gating
// a search means reading and narrowing its predicates, and an interceptor should not have to find
// them in two places. A filter one of them leaves behind is not converted back: the query service
// chooses the outgoing shape from what the reader declared, and a predicate an interceptor added is
// held to the same capability check as one the caller sent.
func (qs QueryService) onTraceQuery(ctx context.Context, query TraceQueryParams) (context.Context, TraceQueryParams, error) {
	queryPreIntercept := toInterceptorTraceQuery(query.ToFilterShape())
	queryPostIntercept := queryPreIntercept
	var err error
	for _, interceptor := range qs.options.Interceptors {
		ctx, queryPostIntercept, err = interceptor.OnTraceQuery(ctx, queryPostIntercept)
		if err != nil {
			return ctx, query, err
		}
	}

	// A legacy query whose predicates no interceptor touched reaches storage in its legacy shape,
	// with only the time range updated in case an interceptor narrowed it. Converting it to the
	// filter shape anyway would change the answer on some backends: Elasticsearch searches a legacy
	// tag over the event location too, while an unqualified filter reference defaults to span or
	// resource only (RFC 0005 §5.1). Enabling an interceptor must not move a result set by itself.
	if query.Filter == nil && reflect.DeepEqual(queryPostIntercept.Filter, queryPreIntercept.Filter) {
		query.StartTimeMin = queryPostIntercept.StartTimeMin
		query.StartTimeMax = queryPostIntercept.StartTimeMax
		return ctx, query, nil
	}

	// Finalized after that comparison, so finalizing's own rewriting cannot read as a change an
	// interceptor made.
	queryPostIntercept.Filter, err = finalizeInterceptorFilter(queryPostIntercept.Filter)
	if err != nil {
		return ctx, query, err
	}
	query.TraceQueryParams = fromInterceptorTraceQuery(queryPostIntercept, query.TraceQueryParams)
	return ctx, query, nil
}

// toInterceptorSpanQuery and fromInterceptorSpanQuery are the span search's converters at the
// same boundary. A span query has one shape, so nothing stays behind on the internal query.
func toInterceptorSpanQuery(q tracestore.SpanQueryParams) queryinterceptor.SpanQuery {
	return queryinterceptor.SpanQuery{
		Filter:       q.Filter,
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
	}
}

func fromInterceptorSpanQuery(q queryinterceptor.SpanQuery) tracestore.SpanQueryParams {
	return tracestore.SpanQueryParams{
		Filter:       q.Filter,
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
	}
}

// onSpanQuery runs every interceptor's OnSpanQuery in order, threading the context each returns
// into the next, as onTraceQuery does for a trace search. There is no legacy branch: the filter
// the interceptors leave behind is finalized and sent on.
func (qs QueryService) onSpanQuery(ctx context.Context, query SpanQueryParams) (context.Context, SpanQueryParams, error) {
	queryPreIntercept := toInterceptorSpanQuery(query.SpanQueryParams)
	queryPostIntercept := queryPreIntercept
	var err error
	for _, interceptor := range qs.options.Interceptors {
		ctx, queryPostIntercept, err = interceptor.OnSpanQuery(ctx, queryPostIntercept)
		if err != nil {
			return ctx, query, err
		}
	}
	// A search over the time range alone has no filter to finalize, and an interceptor that leaves
	// it that way has widened nothing. Any other nil filter is the fail-open case finalizing refuses.
	if queryPreIntercept.Filter != nil || queryPostIntercept.Filter != nil {
		queryPostIntercept.Filter, err = finalizeInterceptorFilter(queryPostIntercept.Filter)
		if err != nil {
			return ctx, query, err
		}
	}
	query.SpanQueryParams = fromInterceptorSpanQuery(queryPostIntercept)
	return ctx, query, nil
}

// finalizeInterceptorFilter finalizes the filter an interceptor returned and rejects what it must
// not hand to storage. An interceptor builds its filter by hand, in code jaeger-query does not
// control, and a malformed tree is typically answered by a backend matching nothing rather than
// refusing — so a search meant to be narrowed would come back wrong with nothing to say why.
//
// It finalizes rather than only validates, so that an interceptor's predicate reaches storage as the
// equal of one a caller sent, having been through the same stage.
//
// A nil filter here is the one mistake that fails open: a search that arrived with predicates and
// leaves with none asks for every trace in the time range. Its caller returns before this point when
// there were no predicates to begin with, since a caller may legitimately search a time range and
// nothing else.
func finalizeInterceptorFilter(returned *expression.Call) (*expression.Call, error) {
	if returned == nil {
		return nil, fmt.Errorf("%w: it returned no filter for a query that had predicates, which "+
			"would widen the search to every trace in the time range", ErrInterceptorFilter)
	}
	finalized, err := tracestore.FinalizeFilter(returned)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInterceptorFilter, err)
	}
	return finalized, nil
}

// interceptTraceResults hands every batch of seq to the interceptors' OnTraceResult in order, threading the
// context each returns into the next so that state can accumulate across a multi-batch result.
// An OnTraceResult error ends the stream rather than yielding later batches, which could leak results
// the failed sanitize or redaction was meant to withhold.
//
// It wraps the batches as storage yielded them, before the query service aggregates and adjusts
// them, so an interceptor rewrites the traces the reader actually returned.
func (qs QueryService) interceptTraceResults(
	ctx context.Context,
	seq iter.Seq2[[]ptrace.Traces, error],
) iter.Seq2[[]ptrace.Traces, error] {
	if len(qs.options.Interceptors) == 0 {
		return seq
	}
	return func(yield func([]ptrace.Traces, error) bool) {
		for traces, err := range seq {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			for _, interceptor := range qs.options.Interceptors {
				ctx, traces, err = interceptor.OnTraceResult(ctx, traces)
				if err != nil {
					yield(nil, err)
					return
				}
			}
			if !yield(traces, nil) {
				return
			}
		}
	}
}

// interceptSpanResults hands every chunk of seq to the interceptors' OnSpanResult in order, with
// the same context threading and error handling as interceptTraceResults. The chunk's page token
// passes through untouched.
func (qs QueryService) interceptSpanResults(
	ctx context.Context,
	seq iter.Seq2[tracestore.PageChunk[ptrace.Traces], error],
) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	if len(qs.options.Interceptors) == 0 {
		return seq
	}
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		for chunk, err := range seq {
			if err != nil {
				if !yield(tracestore.PageChunk[ptrace.Traces]{}, err) {
					return
				}
				continue
			}
			for _, interceptor := range qs.options.Interceptors {
				ctx, chunk.Results, err = interceptor.OnSpanResult(ctx, chunk.Results)
				if err != nil {
					yield(tracestore.PageChunk[ptrace.Traces]{}, err)
					return
				}
			}
			if !yield(chunk, nil) {
				return
			}
		}
	}
}
