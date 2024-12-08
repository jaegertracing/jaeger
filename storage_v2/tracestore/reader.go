// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// ErrTraceNotFound is returned by Reader's GetTrace if no data is found for given trace ID.
var ErrTraceNotFound = spanstore.ErrTraceNotFound

// Reader finds and loads traces and other data from storage.
type Reader interface {
	// GetTrace retrieves the trace with a given id.
	//
	// If no spans are stored for this trace, it returns ErrTraceNotFound.
	GetTrace(ctx context.Context, traceID pcommon.TraceID) (ptrace.Traces, error)

	// GetServices returns all service names known to the backend from spans
	// within its retention period.
	GetServices(ctx context.Context) ([]string, error)

	// GetOperations returns all operation names for a given service
	// known to the backend from spans within its retention period.
	GetOperations(ctx context.Context, query OperationQueryParameters) ([]Operation, error)

	// FindTraces returns all traces matching query parameters. There's currently
	// an implementation-dependent ambiguity whether all query filters (such as
	// multiple tags) must apply to the same span within a trace, or can be satisfied
	// by different spans.
	//
	// If no matching traces are found, the function returns (nil, nil).
	FindTraces(ctx context.Context, query TraceQueryParameters) ([]ptrace.Traces, error)

	// FindTraceIDs does the same search as FindTraces, but returns only the list
	// of matching trace IDs.
	//
	// If no matching traces are found, the function returns (nil, nil).
	FindTraceIDs(ctx context.Context, query TraceQueryParameters) ([]pcommon.TraceID, error)
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
