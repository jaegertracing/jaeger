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

package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go/ext"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Store is an in-memory store of traces
type Store struct {
	sync.RWMutex
	ids        []*model.TraceID
	traces     map[model.TraceID]*model.Trace
	services   map[string]struct{}
	operations map[string]map[string]map[string]struct{}
	deduper    adjuster.Adjuster
	config     config.Configuration
	index      int
}

// NewStore creates an unbounded in-memory store
func NewStore() *Store {
	return WithConfiguration(config.Configuration{MaxTraces: 0})
}

// WithConfiguration creates a new in memory storage based on the given configuration
func WithConfiguration(configuration config.Configuration) *Store {
	return &Store{
		ids:        make([]*model.TraceID, configuration.MaxTraces),
		traces:     map[model.TraceID]*model.Trace{},
		services:   map[string]struct{}{},
		operations: map[string]map[string]map[string]struct{}{},
		deduper:    adjuster.SpanIDDeduper(),
		config:     configuration,
	}
}

// GetDependencies returns dependencies between services
func (m *Store) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	// deduper used below can modify the spans, so we take an exclusive lock
	m.Lock()
	defer m.Unlock()
	deps := map[string]*model.DependencyLink{}
	startTs := endTs.Add(-1 * lookback)
	for _, orig := range m.traces {
		// SpanIDDeduper never returns an err
		trace, _ := m.deduper.Adjust(orig)
		if m.traceIsBetweenStartAndEnd(startTs, endTs, trace) {
			for _, s := range trace.Spans {
				parentSpan := m.findSpan(trace, s.ParentSpanID())
				if parentSpan != nil {
					if parentSpan.Process.ServiceName == s.Process.ServiceName {
						continue
					}
					depKey := parentSpan.Process.ServiceName + "&&&" + s.Process.ServiceName
					if _, ok := deps[depKey]; !ok {
						deps[depKey] = &model.DependencyLink{
							Parent:    parentSpan.Process.ServiceName,
							Child:     s.Process.ServiceName,
							CallCount: 1,
						}
					} else {
						deps[depKey].CallCount++
					}
				}
			}
		}
	}
	retMe := make([]model.DependencyLink, 0, len(deps))
	for _, dep := range deps {
		retMe = append(retMe, *dep)
	}
	return retMe, nil
}

func (m *Store) findSpan(trace *model.Trace, spanID model.SpanID) *model.Span {
	for _, s := range trace.Spans {
		if s.SpanID == spanID {
			return s
		}
	}
	return nil
}

func (m *Store) traceIsBetweenStartAndEnd(startTs, endTs time.Time, trace *model.Trace) bool {
	for _, s := range trace.Spans {
		if s.StartTime.After(startTs) && endTs.After(s.StartTime) {
			return true
		}
	}
	return false
}

// WriteSpan writes the given span
func (m *Store) WriteSpan(span *model.Span) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.operations[span.Process.ServiceName]; !ok {
		m.operations[span.Process.ServiceName] = map[string]map[string]struct{}{}
	}
	if _, ok := m.operations[span.Process.ServiceName][span.OperationName]; !ok {
		m.operations[span.Process.ServiceName][span.OperationName] = map[string]struct{}{}
	}

	spanKind := ""
	if tag, ok := model.KeyValues(span.Tags).FindByKey(string(ext.SpanKind)); ok {
		spanKind = tag.AsString()
	}
	m.operations[span.Process.ServiceName][span.OperationName][spanKind] = struct{}{}
	m.services[span.Process.ServiceName] = struct{}{}
	if _, ok := m.traces[span.TraceID]; !ok {
		m.traces[span.TraceID] = &model.Trace{}

		// if we have a limit, let's cleanup the oldest traces
		if m.config.MaxTraces > 0 {
			// we only have to deal with this slice if we have a limit
			m.index = (m.index + 1) % m.config.MaxTraces

			// do we have an item already on this position? if so, we are overriding it,
			// and we need to remove from the map
			if m.ids[m.index] != nil {
				delete(m.traces, *m.ids[m.index])
			}

			// update the ring with the trace id
			m.ids[m.index] = &span.TraceID
		}

	}
	m.traces[span.TraceID].Spans = append(m.traces[span.TraceID].Spans, span)

	return nil
}

// GetTrace gets a trace
func (m *Store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	m.RLock()
	defer m.RUnlock()
	trace, ok := m.traces[traceID]
	if !ok {
		return nil, spanstore.ErrTraceNotFound
	}
	return m.copyTrace(trace), nil
}

// Spans may still be added to traces after they are returned to user code, so make copies.
func (m *Store) copyTrace(trace *model.Trace) *model.Trace {
	return &model.Trace{
		Spans:    append([]*model.Span(nil), trace.Spans...),
		Warnings: append([]string(nil), trace.Warnings...),
	}
}

// GetServices returns a list of all known services
func (m *Store) GetServices(ctx context.Context) ([]string, error) {
	m.RLock()
	defer m.RUnlock()
	var retMe []string
	for k := range m.services {
		retMe = append(retMe, k)
	}
	return retMe, nil
}

// GetOperations returns the operations of a given service
func (m *Store) GetOperations(ctx context.Context, service string, spanKind string) ([]*storage_v1.OperationMeta, error) {
	m.RLock()
	defer m.RUnlock()
	var retMe []*storage_v1.OperationMeta
	if operations, ok := m.operations[service]; ok {
		for operationName, kinds := range operations {
			for kind := range kinds {
				if spanKind == "" || spanKind == kind {
					retMe = append(retMe, &storage_v1.OperationMeta{
						Operation: operationName,
						SpanKind:  kind,
					})
				}
			}
		}
	}
	return retMe, nil
}

// FindTraces returns all traces in the query parameters are satisfied by a trace's span
func (m *Store) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	m.RLock()
	defer m.RUnlock()
	var retMe []*model.Trace
	for _, trace := range m.traces {
		if m.validTrace(trace, query) {
			retMe = append(retMe, m.copyTrace(trace))
		}
	}

	// Query result order doesn't matter, as the query frontend will sort them anyway.
	// However, if query.NumTraces < results, then we should return the newest traces.
	if query.NumTraces > 0 && len(retMe) > query.NumTraces {
		sort.Slice(retMe, func(i, j int) bool {
			return retMe[i].Spans[0].StartTime.Before(retMe[j].Spans[0].StartTime)
		})
		retMe = retMe[len(retMe)-query.NumTraces:]
	}

	return retMe, nil
}

// FindTraceIDs is not implemented.
func (m *Store) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return nil, errors.New("not implemented")
}

func (m *Store) validTrace(trace *model.Trace, query *spanstore.TraceQueryParameters) bool {
	for _, span := range trace.Spans {
		if m.validSpan(span, query) {
			return true
		}
	}
	return false
}

func findKeyValueMatch(kvs model.KeyValues, key, value string) (model.KeyValue, bool) {
	for _, kv := range kvs {
		if kv.Key == key && kv.AsString() == value {
			return kv, true
		}
	}
	return model.KeyValue{}, false
}

func (m *Store) validSpan(span *model.Span, query *spanstore.TraceQueryParameters) bool {
	if query.ServiceName != span.Process.ServiceName {
		return false
	}
	if query.OperationName != "" && query.OperationName != span.OperationName {
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
	spanKVs := m.flattenTags(span)
	for queryK, queryV := range query.Tags {
		// (NB): we cannot use the KeyValues.FindKey function because there can be multiple tags with the same key
		if _, ok := findKeyValueMatch(spanKVs, queryK, queryV); !ok {
			return false
		}
	}
	return true
}

func (m *Store) flattenTags(span *model.Span) model.KeyValues {
	retMe := span.Tags
	retMe = append(retMe, span.Process.Tags...)
	for _, l := range span.Logs {
		retMe = append(retMe, l.Fields...)
	}
	return retMe
}
