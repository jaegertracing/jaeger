// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptor

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	pub "github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type reader struct {
	next         tracestore.Reader
	interceptors []pub.Interceptor
}

// NewReaderDecorator decorates next so the given interceptors are applied around
// every trace query: OnQuery on the query parameters of FindTraces,
// FindTraceIDs, and FindTraceSummaries, and OnResult on every batch of traces
// yielded by FindTraces and GetTraces. Interceptors run in the order given.
// Callers wrap only when there is at least one interceptor, so this always
// returns a decorator.
//
// The interceptors see the public queryinterceptor.Query; this decorator
// converts to and from the internal tracestore.TraceQueryParams at the boundary,
// so the internal query type never crosses the contract.
func NewReaderDecorator(next tracestore.Reader, interceptors ...pub.Interceptor) tracestore.Reader {
	return &reader{next: next, interceptors: interceptors}
}

func toPublicQuery(q tracestore.TraceQueryParams) pub.Query {
	return pub.Query{
		ServiceName:   q.ServiceName,
		OperationName: q.OperationName,
		Attributes:    q.Attributes,
		StartTimeMin:  q.StartTimeMin,
		StartTimeMax:  q.StartTimeMax,
		DurationMin:   q.DurationMin,
		DurationMax:   q.DurationMax,
		SearchDepth:   q.SearchDepth,
	}
}

func fromPublicQuery(q pub.Query) tracestore.TraceQueryParams {
	return tracestore.TraceQueryParams{
		ServiceName:   q.ServiceName,
		OperationName: q.OperationName,
		Attributes:    q.Attributes,
		StartTimeMin:  q.StartTimeMin,
		StartTimeMax:  q.StartTimeMax,
		DurationMin:   q.DurationMin,
		DurationMax:   q.DurationMax,
		SearchDepth:   q.SearchDepth,
	}
}

// onQuery runs every interceptor's OnQuery in order, threading the context each
// returns into the next. The final context is returned so callers can pass it to
// the storage reader and to onResult, letting an interceptor carry per-query state
// (e.g. a resolved caller identity) from the pre-query hook to the return path.
func (r *reader) onQuery(ctx context.Context, query tracestore.TraceQueryParams) (context.Context, tracestore.TraceQueryParams, error) {
	pq := toPublicQuery(query)
	var err error
	for _, interceptor := range r.interceptors {
		ctx, pq, err = interceptor.OnQuery(ctx, pq)
		if err != nil {
			return ctx, query, err
		}
	}
	return ctx, fromPublicQuery(pq), nil
}

// onResult runs every interceptor's OnResult in order on one batch, threading the
// context each returns into the next. The final context is returned so the caller
// can feed it into onResult for the next batch, letting state accumulate across a
// multi-batch result.
func (r *reader) onResult(ctx context.Context, traces []ptrace.Traces) (context.Context, []ptrace.Traces, error) {
	var err error
	for _, interceptor := range r.interceptors {
		ctx, traces, err = interceptor.OnResult(ctx, traces)
		if err != nil {
			return ctx, nil, err
		}
	}
	return ctx, traces, nil
}

func (r *reader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		var err error
		// onQuery returns the (possibly enriched) context and gated query; assign
		// them back so every statement below sees the post-hook values, never the
		// originals. The same applies to onResult's context inside the loop.
		ctx, query, err = r.onQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		for traces, err := range r.next.FindTraces(ctx, query) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			ctx, traces, err = r.onResult(ctx, traces)
			if err != nil {
				// Per the Interceptor contract, an OnResult error aborts the stream:
				// stop rather than emit further batches, which could leak results the
				// failed sanitize/redaction was meant to withhold.
				yield(nil, err)
				return
			}
			if !yield(traces, nil) {
				return
			}
		}
	}
}

func (r *reader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		var err error
		ctx, query, err = r.onQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		for ids, err := range r.next.FindTraceIDs(ctx, query) {
			if !yield(ids, err) {
				return
			}
		}
	}
}

func (r *reader) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		// GetTraces has no pre-query hook, so OnResult's context chain seeds from
		// the inbound context; assigning it back threads across batches.
		for traces, err := range r.next.GetTraces(ctx, traceIDs...) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			ctx, traces, err = r.onResult(ctx, traces)
			if err != nil {
				// Per the Interceptor contract, an OnResult error aborts the stream:
				// stop rather than emit further batches, which could leak results the
				// failed sanitize/redaction was meant to withhold.
				yield(nil, err)
				return
			}
			if !yield(traces, nil) {
				return
			}
		}
	}
}

func (r *reader) GetServices(ctx context.Context) ([]string, error) {
	return r.next.GetServices(ctx)
}

func (r *reader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return r.next.GetOperations(ctx, query)
}

// FindTraceSummaries forwards to the wrapped reader's FindTraceSummaries,
// applying OnQuery gating to the search. When the wrapped reader has no native
// summary support it yields errors.ErrUnsupported and the query service falls
// back to FindTraces (which this decorator also gates). OnResult is not applied
// here: summaries carry only per-trace metadata (not spans), and the Interceptor
// contract has no summary hook — so an interceptor gates/constrains the summary
// search via OnQuery, but cannot drop or redact individual summaries. A dedicated
// summary hook is left as a contract extension.
func (r *reader) FindTraceSummaries(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		var err error
		ctx, query, err = r.onQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		for summaries, err := range r.next.FindTraceSummaries(ctx, query) {
			if !yield(summaries, err) {
				return
			}
		}
	}
}
