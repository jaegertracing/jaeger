// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	v2api "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const noServiceName = "OTLPResourceNoServiceName"

// Store is an in-memory store of traces
type Store struct {
	sync.RWMutex
	// Each tenant gets a copy of default config.
	// In the future this can be extended to contain per-tenant configuration.
	defaultConfig v1.Configuration
	perTenant     map[string]*Tenant
}

// Tenant is an in-memory store of traces for a single tenant
type Tenant struct {
	sync.RWMutex
	ids        []pcommon.TraceID
	traces     map[pcommon.TraceID]ptrace.Traces
	services   map[string]struct{}
	operations map[string]map[v2api.Operation]struct{}
	config     v1.Configuration
	index      int
}

// NewStore creates an unbounded in-memory store
func NewStore() *Store {
	return WithConfiguration(v1.Configuration{MaxTraces: 0})
}

// WithConfiguration creates a new in memory storage based on the given configuration
func WithConfiguration(cfg v1.Configuration) *Store {
	return &Store{
		defaultConfig: cfg,
		perTenant:     make(map[string]*Tenant),
	}
}

func newTenant(cfg v1.Configuration) *Tenant {
	return &Tenant{
		ids:        make([]pcommon.TraceID, cfg.MaxTraces),
		traces:     map[pcommon.TraceID]ptrace.Traces{},
		services:   map[string]struct{}{},
		operations: map[string]map[v2api.Operation]struct{}{},
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

// WriteTraces write the traces into the tenant by grouping all the spans with same trace id together.
// The traces will not be saved as they are coming, rather they would be reshuffled, to understand this reshuffling
// take an example of traces which have 2 resource spans, then these two resource spans have 2 scope spans each.
// Every scope span consists of 2 spans with trace ids: 1 and 2. Now the final structure should look like:
// For TraceID1: [ResourceSpan1:[ScopeSpan1:[Span(TraceID1)],ScopeSpan2:[Span(TraceID1)], ResourceSpan2:[ScopeSpan1:[Span(TraceID1)],ScopeSpan2:[Span(TraceID1)]
// A similar structure will be there for TraceID2
func (st *Store) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.Lock()
	defer m.Unlock()
	sameTraceIDResourceSpans := make(map[pcommon.TraceID]ptrace.ResourceSpansSlice)
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		sameTraceIDScopeSpans := make(map[pcommon.TraceID]ptrace.ScopeSpansSlice)
		resourceSpan := td.ResourceSpans().At(i)
		reshuffleScopeSpanAndSaveServiceAndOperation(m, resourceSpan, sameTraceIDScopeSpans)
		// All the  scope spans here will have the same resource as of resourceSpan. Therefore:
		// Copy the resource to an empty resourceSpan. After this, append the scope spans with same
		// trace id to this empty resource span. Finally move this resource span to the resourceSpanSlice
		// containing other resource spans and having same trace id.
		for traceId, sameTraceIDsScopeSpansSlice := range sameTraceIDScopeSpans {
			sameTraceIDResourceSpan := ptrace.NewResourceSpans()
			resourceSpan.Resource().CopyTo(sameTraceIDResourceSpan.Resource())
			sameTraceIDsScopeSpansSlice.MoveAndAppendTo(sameTraceIDResourceSpan.ScopeSpans())
			if sameTraceIDResourceSpanSlice, ok := sameTraceIDResourceSpans[traceId]; ok {
				sameTraceIDResourceSpan.MoveTo(sameTraceIDResourceSpanSlice.AppendEmpty())
			} else {
				resourceSpanSlice := ptrace.NewResourceSpansSlice()
				sameTraceIDResourceSpan.MoveTo(resourceSpanSlice.AppendEmpty())
				sameTraceIDResourceSpans[traceId] = resourceSpanSlice
			}
		}
	}
	for traceId, resourceSpansSlice := range sameTraceIDResourceSpans {
		traces := m.traces[traceId] // Ignoring the ok, as that is not possible because this check is already made when trace limits are used to do cleanup
		resourceSpansSlice.MoveAndAppendTo(traces.ResourceSpans())
	}
	return nil
}

// reshuffleScopeSpanAndSaveServiceAndOperation reshuffles all the scope spans of a resource span to group
// spans of same trace ids together. The first step is to iterate the scope spans and then.
// copy the scope to an empty scopeSpan. After this, append the spans with same
// trace id to this empty scope span. Finally move this scope span to the scope span
// slice containing other scope spans and having same trace id.
func reshuffleScopeSpanAndSaveServiceAndOperation(m *Tenant, resourceSpan ptrace.ResourceSpans, sameTraceIDScopeSpans map[pcommon.TraceID]ptrace.ScopeSpansSlice) {
	serviceName := getServiceNameFromResource(resourceSpan.Resource())
	m.services[serviceName] = struct{}{}
	for j := 0; j < resourceSpan.ScopeSpans().Len(); j++ {
		sameTraceIDSpans := make(map[pcommon.TraceID]ptrace.SpanSlice)
		scopeSpan := resourceSpan.ScopeSpans().At(j)
		reshuffleSpansAndSaveOperation(m, serviceName, scopeSpan, sameTraceIDSpans)
		for traceId, sameTraceIDSpanSlice := range sameTraceIDSpans {
			sameTraceIDScopeSpan := ptrace.NewScopeSpans()
			scopeSpan.Scope().CopyTo(sameTraceIDScopeSpan.Scope())
			sameTraceIDSpanSlice.MoveAndAppendTo(sameTraceIDScopeSpan.Spans())
			if sameTraceIDScopeSpanSlice, ok := sameTraceIDScopeSpans[traceId]; ok {
				sameTraceIDScopeSpan.MoveTo(sameTraceIDScopeSpanSlice.AppendEmpty())
			} else {
				scopeSpanSlice := ptrace.NewScopeSpansSlice()
				sameTraceIDScopeSpan.MoveTo(scopeSpanSlice.AppendEmpty())
				sameTraceIDScopeSpans[traceId] = scopeSpanSlice
			}
		}
	}
}

func reshuffleSpansAndSaveOperation(m *Tenant, serviceName string, scopeSpan ptrace.ScopeSpans, sameTraceIDSpans map[pcommon.TraceID]ptrace.SpanSlice) {
	for k := 0; k < scopeSpan.Spans().Len(); k++ {
		span := scopeSpan.Spans().At(k)
		operation := getOperationFromSpanNameAndKind(span.Name(), span.Kind())
		saveOperationToStore(m, serviceName, operation)
		if _, ok := m.traces[span.TraceID()]; !ok {
			m.traces[span.TraceID()] = ptrace.NewTraces()
			// if we have a limit, let's cleanup the oldest traces
			if m.config.MaxTraces > 0 {
				// we only have to deal with this slice if we have a limit
				m.index = (m.index + 1) % m.config.MaxTraces
				// do we have an item already on this position? if so, we are overriding it,
				// and we need to remove from the map
				if !m.ids[m.index].IsEmpty() {
					delete(m.traces, m.ids[m.index])
				}
				// update the ring with the trace id
				m.ids[m.index] = span.TraceID()
			}
		}
		// Collect all the spans with same trace id within the same scope span sameTraceIDSpanSlice
		if sameTraceIDSpanSlice, ok := sameTraceIDSpans[span.TraceID()]; ok {
			span.CopyTo(sameTraceIDSpanSlice.AppendEmpty())
		} else {
			spanSlice := ptrace.NewSpanSlice()
			span.CopyTo(spanSlice.AppendEmpty())
			sameTraceIDSpans[span.TraceID()] = spanSlice
		}
	}
}

func saveOperationToStore(m *Tenant, serviceName string, operation v2api.Operation) {
	if _, ok := m.operations[serviceName]; !ok {
		m.operations[serviceName] = make(map[v2api.Operation]struct{})
		m.operations[serviceName][operation] = struct{}{}
	} else {
		m.operations[serviceName][operation] = struct{}{}
	}
}

func getOperationFromSpanNameAndKind(name string, kind ptrace.SpanKind) v2api.Operation {
	return v2api.Operation{
		Name:     name,
		SpanKind: kind.String(),
	}
}

func getServiceNameFromResource(resource pcommon.Resource) string {
	val, ok := resource.Attributes().Get(conventions.AttributeServiceName)
	if !ok {
		return noServiceName
	}
	serviceName := val.Str()
	if serviceName == "" {
		return noServiceName
	}
	return serviceName
}
