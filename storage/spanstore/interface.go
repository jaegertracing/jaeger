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

// Writer writes spans to storage.
type Writer interface {
	WriteSpan(span *model.Span) error
}

var (
	// ErrTraceNotFound is returned by Reader's GetTrace if no data is found for given trace ID.
	ErrTraceNotFound = errors.New("trace not found")
)

// Reader finds and loads traces and other data from storage.
type Reader interface {
	GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error)
	GetServices(ctx context.Context) ([]string, error)
	GetOperations(ctx context.Context, query OperationQueryParameters) ([]Operation, error)
	FindTraces(ctx context.Context, query *TraceQueryParameters) ([]*model.Trace, error)
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
