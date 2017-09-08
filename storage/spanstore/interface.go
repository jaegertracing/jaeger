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
	"errors"
	"time"

	"github.com/uber/jaeger/model"
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
	GetTrace(traceID model.TraceID) (*model.Trace, error)
	GetServices() ([]string, error)
	GetOperations(service string) ([]string, error)
	FindTraces(query *TraceQueryParameters) ([]*model.Trace, error)
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
