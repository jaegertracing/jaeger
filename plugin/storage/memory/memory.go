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

	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Store is an in-memory store of traces
type Store struct {
	sync.RWMutex
	// Each tenant gets a copy of default config.
	// In the future this can be extended to contain per-tenant configuration.
	defaultConfig config.Configuration
	perTenant     map[string]*Tenant
}

// Tenant is an in-memory store of traces for a single tenant
type Tenant struct {
	sync.RWMutex
	ids        []*model.TraceID
	traces     map[model.TraceID]*model.Trace
	services   map[string]struct{}
	operations map[string]map[spanstore.Operation]struct{}
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
		defaultConfig: configuration,
		perTenant:     make(map[string]*Tenant),
	}
}

func newTenant(cfg config.Configuration) *Tenant {
	return &Tenant{
		ids:        make([]*model.TraceID, cfg.MaxTraces),
		traces:     map[model.TraceID]*model.Trace{},
		services:   map[string]struct{}{},
		operations: map[string]map[spanstore.Operation]struct{}{},
		deduper:    adjuster.SpanIDDeduper(),
		config:     cfg,
	}
}

// getTenant returns the per-tenant storage.  Note that tenantID has already been checked for by the collector or query
func (st *Store) getTenant(tenantID string) *Tenant {
	st.RLock()
	tenant, ok := st.perTenant[tenantID]
	st.RUnlock()
	if !ok {
		st.Lock()
		defer st.Unlock()
		tenant, ok = st.perTenant[tenantID]
		if !ok {
			tenant = newTenant(st.defaultConfig)
			st.perTenant[tenantID] = tenant
		}
	}
	return tenant
}

// GetDependencies returns dependencies between services
func (st *Store) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	// deduper used below can modify the spans, so we take an exclusive lock
	m.Lock()
	defer m.Unlock()
	deps := map[string]*model.DependencyLink{}
	startTs := endTs.Add(-1 * lookback)
	for _, orig := range m.traces {
		// SpanIDDeduper never returns an err
		trace, _ := m.deduper.Adjust(orig)
		if traceIsBetweenStartAndEnd(startTs, endTs, trace) {
			for _, s := range trace.Spans {
				parentSpan := findSpan(trace, s.ParentSpanID())
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

func findSpan(trace *model.Trace, spanID model.SpanID) *model.Span {
	for _, s := range trace.Spans {
		if s.SpanID == spanID {
			return s
		}
	}
	return nil
}

func traceIsBetweenStartAndEnd(startTs, endTs time.Time, trace *model.Trace) bool {
	for _, s := range trace.Spans {
		if s.StartTime.After(startTs) && endTs.After(s.StartTime) {
			return true
		}
	}
	return false
}

// WriteSpan writes the given span
func (st *Store) WriteSpan(ctx context.Context, span *model.Span) error {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.Lock()
	defer m.Unlock()
	if _, ok := m.operations[span.Process.ServiceName]; !ok {
		m.operations[span.Process.ServiceName] = map[spanstore.Operation]struct{}{}
	}

	spanKind, _ := span.GetSpanKind()
	operation := spanstore.Operation{
		Name:     span.OperationName,
		SpanKind: spanKind.String(),
	}

	if _, ok := m.operations[span.Process.ServiceName][operation]; !ok {
		m.operations[span.Process.ServiceName][operation] = struct{}{}
	}

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
func (st *Store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.RLock()
	defer m.RUnlock()
	trace, ok := m.traces[traceID]
	if !ok {
		return nil, spanstore.ErrTraceNotFound
	}
	return copyTrace(trace)
}

// Spans may still be added to traces after they are returned to user code, so make copies.
func copyTrace(trace *model.Trace) (*model.Trace, error) {
	bytes, err := proto.Marshal(trace)
	if err != nil {
		return nil, err
	}

	copied := &model.Trace{}
	err = proto.Unmarshal(bytes, copied)
	return copied, err
}

// GetServices returns a list of all known services
func (st *Store) GetServices(ctx context.Context) ([]string, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.RLock()
	defer m.RUnlock()
	var retMe []string
	for k := range m.services {
		retMe = append(retMe, k)
	}
	return retMe, nil
}

// GetOperations returns the operations of a given service
func (st *Store) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.RLock()
	defer m.RUnlock()
	var retMe []spanstore.Operation
	if operations, ok := m.operations[query.ServiceName]; ok {
		for operation := range operations {
			if query.SpanKind == "" || query.SpanKind == operation.SpanKind {
				retMe = append(retMe, operation)
			}
		}
	}
	return retMe, nil
}

// FindTraces returns all traces in the query parameters are satisfied by a trace's span
func (st *Store) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.RLock()
	defer m.RUnlock()
	var retMe []*model.Trace
	for _, trace := range m.traces {
		if validTrace(trace, query) {
			copied, err := copyTrace(trace)
			if err != nil {
				return nil, err
			}

			retMe = append(retMe, copied)
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

func validTrace(trace *model.Trace, query *spanstore.TraceQueryParameters) bool {
	for _, span := range trace.Spans {
		if validSpan(span, query) {
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

func validSpan(span *model.Span, query *spanstore.TraceQueryParameters) bool {
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
	spanKVs := flattenTags(span)
	for queryK, queryV := range query.Tags {
		// (NB): we cannot use the KeyValues.FindKey function because there can be multiple tags with the same key
		if _, ok := findKeyValueMatch(spanKVs, queryK, queryV); !ok {
			return false
		}
	}
	return true
}

func flattenTags(span *model.Span) model.KeyValues {
	retMe := []model.KeyValue{}
	retMe = append(retMe, span.Tags...)
	retMe = append(retMe, span.Process.Tags...)
	for _, l := range span.Logs {
		retMe = append(retMe, l.Fields...)
	}
	return retMe
}
