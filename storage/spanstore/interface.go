// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanstore

import (
	"context"
	"errors"
	"time"

	"github.com/jaegertracing/jaeger/model"
)

var (
	// ErrTraceNotFound is returned by Reader's GetTrace if no data is found for given trace ID.
	ErrTraceNotFound = errors.New("trace not found")
)

// Writer writes spans to storage.
type Writer interface {
	WriteSpan(ctx context.Context, span *model.Span) error
}

// Reader finds and loads traces and other data from storage.
type Reader interface {
	// GetTrace retrieves the trace with a given id.
	//
	// If no spans are stored for this trace, it returns ErrTraceNotFound.
	GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error)

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
