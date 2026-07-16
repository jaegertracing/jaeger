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

// NewReaderDecorator decorates next so the given interceptors are applied around every
// trace query: OnQuery on the query parameters of FindTraces and FindTraceIDs,
// and OnResult on every batch of traces yielded by FindTraces and GetTraces.
// Interceptors run in the order given. With no interceptors, next is returned
// unchanged so the default read path has zero overhead.
//
// The interceptors see the public queryinterceptor.Query; this decorator
// converts to and from the internal tracestore.TraceQueryParams at the boundary,
// so the internal query type never crosses the contract.
func NewReaderDecorator(next tracestore.Reader, interceptors ...pub.Interceptor) tracestore.Reader {
	if len(interceptors) == 0 {
		return next
	}
	return &reader{next: next, interceptors: interceptors}
}

type reader struct {
	next         tracestore.Reader
	interceptors []pub.Interceptor
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

func (r *reader) onQuery(ctx context.Context, query tracestore.TraceQueryParams) (tracestore.TraceQueryParams, error) {
	pq := toPublicQuery(query)
	var err error
	for _, interceptor := range r.interceptors {
		pq, err = interceptor.OnQuery(ctx, pq)
		if err != nil {
			return query, err
		}
	}
	return fromPublicQuery(pq), nil
}

func (r *reader) onResult(ctx context.Context, traces []ptrace.Traces) ([]ptrace.Traces, error) {
	var err error
	for _, interceptor := range r.interceptors {
		traces, err = interceptor.OnResult(ctx, traces)
		if err != nil {
			return nil, err
		}
	}
	return traces, nil
}

func (r *reader) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		gated, err := r.onQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		for traces, err := range r.next.FindTraces(ctx, gated) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			sanitized, serr := r.onResult(ctx, traces)
			if serr != nil {
				if !yield(nil, serr) {
					return
				}
				continue
			}
			if !yield(sanitized, nil) {
				return
			}
		}
	}
}

func (r *reader) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		gated, err := r.onQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		for ids, err := range r.next.FindTraceIDs(ctx, gated) {
			if !yield(ids, err) {
				return
			}
		}
	}
}

func (r *reader) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for traces, err := range r.next.GetTraces(ctx, traceIDs...) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			sanitized, serr := r.onResult(ctx, traces)
			if serr != nil {
				if !yield(nil, serr) {
					return
				}
				continue
			}
			if !yield(sanitized, nil) {
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
