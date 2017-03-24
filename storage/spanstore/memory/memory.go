// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package memory

import (
	"sync"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
	"time"
)

// Store is an in-memory store of traces
type Store struct {
	sync.RWMutex
	traces     map[model.TraceID]*model.Trace
	services   map[string]struct{}
	operations map[string]map[string]struct{}
}

// NewStore creates an in-memory store
func NewStore() *Store {
	return &Store{
		traces:     map[model.TraceID]*model.Trace{},
		services:   map[string]struct{}{},
		operations: map[string]map[string]struct{}{},
	}
}

// GetDependencies returns dependencies between services
func (m *Store) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	// TODO - go through all traces and build dependencies between start and end time
	return nil, nil
}

// WriteSpan writes the given span
func (m *Store) WriteSpan(span *model.Span) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.operations[span.Process.ServiceName]; !ok {
		m.operations[span.Process.ServiceName] = map[string]struct{}{}
	}
	m.operations[span.Process.ServiceName][span.OperationName] = struct{}{}
	m.services[span.Process.ServiceName] = struct{}{}
	if _, ok := m.traces[span.TraceID]; !ok {
		m.traces[span.TraceID] = &model.Trace{}
	}
	m.traces[span.TraceID].Spans = append(m.traces[span.TraceID].Spans, span)
	return nil
}

// GetTrace gets a trace
func (m *Store) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	m.RLock()
	defer m.RUnlock()
	return m.traces[traceID], nil
}

// GetServices returns a list of all known services
func (m *Store) GetServices() ([]string, error) {
	m.RLock()
	defer m.RUnlock()
	var retMe []string
	for k := range m.services {
		retMe = append(retMe, k)
	}
	return retMe, nil
}

// GetOperations returns the operations of a given service
func (m *Store) GetOperations(service string) ([]string, error) {
	m.RLock()
	defer m.RUnlock()
	if operations, ok := m.operations[service]; ok {
		var retMe []string
		for ops := range operations {
			retMe = append(retMe, ops)
		}
		return retMe, nil
	}
	return nil, nil
}

// FindTraces returns all traces in the query parameters are satisfied by a trace's span
func (m *Store) FindTraces(query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	var retMe []*model.Trace
	for _, trace := range m.traces {
		if len(retMe) >= query.NumTraces {
			return retMe, nil
		}
		if m.validTrace(trace, query) {
			retMe = append(retMe, trace)
		}
	}
	return retMe, nil
}

func (m *Store) validTrace(trace *model.Trace, query *spanstore.TraceQueryParameters) bool {
	for _, span := range trace.Spans {
		if m.validSpan(span, query) {
			return true
		}
	}
	return false
}

func (m *Store) validSpan(span *model.Span, query *spanstore.TraceQueryParameters) bool {
	if query.ServiceName != span.Process.ServiceName {
		return false
	}
	if query.OperationName != span.OperationName {
		return false
	}
	if query.DurationMin != 0 && span.Duration < query.DurationMin {
		return false
	}
	if query.DurationMax != 0 && span.Duration > query.DurationMax {
		return false
	}
	if !query.StartTimeMin.IsZero() && span.StartTime.Before(query.StartTimeMin) {
		return false
	}
	if !query.StartTimeMax.IsZero() && span.StartTime.After(query.StartTimeMax) {
		return false
	}
	for queryK, queryV := range query.Tags {
		keyValueFoundAndMatches := false
		for _, keyValue := range span.Tags {
			if keyValue.Key == queryK {
				if keyValue.AsString() != queryV {
					return false
				}
				keyValueFoundAndMatches = true
				break
			}
		}
		if !keyValueFoundAndMatches {
			return false
		}
	}
	return true
}
