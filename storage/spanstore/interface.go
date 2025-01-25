// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// ErrTraceNotFound is returned by Reader's GetTrace if no data is found for given trace ID.
var ErrTraceNotFound = errors.New("trace not found")

// Writer writes spans to storage.
type Writer interface {
	WriteSpan(ctx context.Context, span *model.Span) error
}

// Reader finds and loads traces and other data from storage.
type Reader interface {
	// GetTrace retrieves the trace with a given id.
	//
	// If no spans are stored for this trace, it returns ErrTraceNotFound.
	GetTrace(ctx context.Context, query GetTraceParameters) (*model.Trace, error)

	// GetServices returns all service names known to the backend from spans
	// within its retention period.
	GetServices(ctx context.Context) ([]string, error)

	// GetOperations returns all operation names for a given service
	// known to the backend from spans within its retention period.
	GetOperations(ctx context.Context, query OperationQueryParameters) ([]Operation, error)

	// FindTraces returns all traces matching query parameters. There's currently
	// an implementation-dependent abiguity whether all query filters (such as
	// multiple tags) must apply to the same span within a trace, or can be satisfied
	// by different spans.
	//
	// If no matching traces are found, the function returns (nil, nil).
	FindTraces(ctx context.Context, query *TraceQueryParameters) ([]*model.Trace, error)

	// FindTraceIDs does the same search as FindTraces, but returns only the list
	// of matching trace IDs.
	//
	// If no matching traces are found, the function returns (nil, nil).
	FindTraceIDs(ctx context.Context, query *TraceQueryParameters) ([]model.TraceID, error)
}

// GetTraceParameters contains parameters of a trace get.
type GetTraceParameters struct {
	TraceID   model.TraceID
	StartTime time.Time // optional
	EndTime   time.Time // optional
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
