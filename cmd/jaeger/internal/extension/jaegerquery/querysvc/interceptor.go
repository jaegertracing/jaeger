// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"iter"

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

// toPublicQuery and fromPublicQuery convert at the contract boundary, so the internal query type
// never crosses it. Only the envelope and the filter survive the round trip, which is all the
// public Query carries: onQuery hands over a query whose predicate fields are already empty.
func toPublicQuery(q tracestore.TraceQueryParams) queryinterceptor.Query {
	return queryinterceptor.Query{
		Filter:       q.Filter,
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
		SearchDepth:  q.SearchDepth,
	}
}

func fromPublicQuery(q queryinterceptor.Query) tracestore.TraceQueryParams {
	return tracestore.TraceQueryParams{
		Attributes:   pcommon.NewMap(),
		Filter:       q.Filter,
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
		SearchDepth:  q.SearchDepth,
	}
}

// onQuery runs every interceptor's OnQuery in order, threading the context each returns into the
// next. The final context is returned so the caller can pass it to the storage reader and to
// OnResult, letting an interceptor carry per-query state (a resolved caller identity, say) from
// the pre-query hook to the return path.
//
// The interceptors are shown the query in filter shape whatever shape it arrived in, because
// gating a search means reading and narrowing its predicates and an interceptor should not have
// to find them in two places. Nothing converts it back: the query service then chooses the
// outgoing shape from what the reader declared, and a predicate an interceptor added is held to
// the same capability check as one the caller sent.
func (qs QueryService) onQuery(ctx context.Context, query TraceQueryParams) (context.Context, TraceQueryParams, error) {
	asked := query
	asked.TraceQueryParams = query.ToFilterShape()
	public := toPublicQuery(asked.TraceQueryParams)
	var err error
	for _, interceptor := range qs.options.Interceptors {
		ctx, public, err = interceptor.OnQuery(ctx, public)
		if err != nil {
			return ctx, query, err
		}
	}
	finalized, err := finalizeInterceptorFilter(public.Filter, asked.Filter)
	if err != nil {
		return ctx, query, err
	}
	public.Filter = finalized
	asked.TraceQueryParams = fromPublicQuery(public)
	return ctx, asked, nil
}

// finalizeInterceptorFilter finalizes what an interceptor returns and rejects what it must not
// hand to storage. An interceptor builds its filter by hand, in code jaeger-query does not control,
// and a malformed tree is typically answered by a backend matching nothing rather than refusing —
// so a search meant to be narrowed would come back wrong with nothing to say why.
//
// Finalizing rather than only checking is what makes an interceptor's predicate the equal of a
// caller's: `span.duration > "2s"` added here arrives at a backend as a length of time, the same as
// one that came in over api_v3. Running it a second time on a filter the caller already sent
// changes nothing, since finalizing is idempotent.
//
// Dropping the filter is refused separately, because it is the one mistake that fails open: a
// search that arrived with predicates and leaves with none asks for every trace in the range.
// It is only a mistake when there was something to drop, since a caller may legitimately search
// a time range and nothing else.
func finalizeInterceptorFilter(returned, asked *expression.Call) (*expression.Call, error) {
	if returned == nil {
		if asked == nil {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: it returned no filter for a query that had predicates, which "+
			"would widen the search to every trace in the time range", ErrInterceptorFilter)
	}
	finalized, err := expression.Finalize(returned)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInterceptorFilter, err)
	}
	return finalized, nil
}

// interceptResults hands every batch of seq to the interceptors' OnResult in order, threading the
// context each returns into the next so that state can accumulate across a multi-batch result.
// An OnResult error ends the stream rather than yielding later batches, which could leak results
// the failed sanitize or redaction was meant to withhold.
//
// It wraps the batches as storage yielded them, before the query service aggregates and adjusts
// them, so an interceptor rewrites the traces the reader actually returned.
func (qs QueryService) interceptResults(
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
				ctx, traces, err = interceptor.OnResult(ctx, traces)
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
