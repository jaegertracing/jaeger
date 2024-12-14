// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Reader finds and loads traces and other data from storage.
type Reader interface {
	// GetTrace returns an iterator that retrieves all spans of the trace with a given id.
	// The iterator is single-use: once consumed, it cannot be used again.
	//
	// If the trace is too large it may be returned in multiple chunks.
	//
	// If no spans are stored for this trace, it returns an empty iterator.
	GetTrace(ctx context.Context, traceID pcommon.TraceID) iter.Seq2[ptrace.Traces, error]

	// GetServices returns all service names known to the backend from spans
	// within its retention period.
	GetServices(ctx context.Context) ([]string, error)

	// GetOperations returns all operation names for a given service
	// known to the backend from spans within its retention period.
	GetOperations(ctx context.Context, query OperationQueryParameters) ([]Operation, error)

	// FindTraces returns an iterator that retrieves traces matching query parameters.
	// The iterator is single-use: once consumed, it cannot be used again.
	//
	// There is no guarantee that all spans for a single trace are returned in a single chunk
	// (same as GetTrace: it the trace is too large it may be returned in multiple chunks).
	// However, it is guaranteed that all spans for a single trace are returned in
	// one or more consecutive chunks, as if the total output is grouped by trace ID.
	//
	// If no matching traces are found, the function returns an empty iterator.
	//
	// There's currently an implementation-dependent ambiguity whether all query filters
	// (such as multiple tags) must apply to the same span within a trace, or can be satisfied
	// by different spans.
	FindTraces(ctx context.Context, query TraceQueryParameters) iter.Seq2[[]ptrace.Traces, error]

	// FindTraceIDs returns an iterator that retrieves IDs of traces matching query parameters.
	// The iterator is single-use: once consumed, it cannot be used again.
	//
	// If no matching traces are found, the function returns an empty iterator.
	//
	// This function behaves identically to FindTraces, except that it returns only the list
	// of matching trace IDs. This is useful in some contexts, such as batch jobs, where a
	// large list of trace IDs may be queried first and then the full traces are loaded
	// in batches.
	FindTraceIDs(ctx context.Context, query TraceQueryParameters) iter.Seq2[[]pcommon.TraceID, error]
}

// TraceQueryParameters contains parameters of a trace query.
type TraceQueryParameters struct {
	ServiceName   string
	OperationName string
	Tags          map[string]string
	StartTimeMin  time.Time
	StartTimeMax  time.Time
	DurationMin   time.Duration
	DurationMax   time.Duration
	NumTraces     int
}

func (t *TraceQueryParameters) ToSpanStoreQueryParameters() *spanstore.TraceQueryParameters {
	return &spanstore.TraceQueryParameters{
		ServiceName:   t.ServiceName,
		OperationName: t.OperationName,
		Tags:          t.Tags,
		StartTimeMin:  t.StartTimeMin,
		StartTimeMax:  t.StartTimeMax,
		DurationMin:   t.DurationMin,
		DurationMax:   t.DurationMax,
		NumTraces:     t.NumTraces,
	}
}

// OperationQueryParameters contains parameters of query operations, empty spanKind means get operations for all kinds of span.
type OperationQueryParameters struct {
	ServiceName string
	SpanKind    string
}

// Operation contains operation name and span kind
type Operation struct {
	Name     string
	SpanKind string
}
