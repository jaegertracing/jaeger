// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptor

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// NewReader decorates next so that the given interceptors are applied around
// every trace query: OnQuery on the query parameters of FindTraces and
// FindTraceIDs, and OnResult on every batch of traces yielded by FindTraces and
// GetTraces. Interceptors run in the order given. When no interceptors are
// supplied, next is returned unchanged so there is zero overhead on the
// default (no-interceptor) path.
//
// This decorator is the whole of the change on the query-service side: the
// QueryService keeps calling a tracestore.Reader and is unaware that the reader
// now consults the interceptors.
func NewReader(next tracestore.Reader, interceptors ...Interceptor) tracestore.Reader {
	if len(interceptors) == 0 {
		return next
	}
	return &reader{next: next, interceptors: interceptors}
}

type reader struct {
	next         tracestore.Reader
	interceptors []Interceptor
}

func (r *reader) onQuery(ctx context.Context, query tracestore.TraceQueryParams) (tracestore.TraceQueryParams, error) {
	var err error
	for _, interceptor := range r.interceptors {
		query, err = interceptor.OnQuery(ctx, query)
		if err != nil {
			return query, err
		}
	}
	return query, nil
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
