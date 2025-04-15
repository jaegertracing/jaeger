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
	config     *v1.Configuration
	index      int
}

func (t *Tenant) storeService(serviceName string) {
	t.Lock()
	defer t.Unlock()
	t.services[serviceName] = struct{}{}
}

func (t *Tenant) storeOperation(serviceName string, operation v2api.Operation) {
	t.Lock()
	defer t.Unlock()
	if _, ok := t.operations[serviceName]; !ok {
		t.operations[serviceName] = make(map[v2api.Operation]struct{})
		t.operations[serviceName][operation] = struct{}{}
	} else {
		t.operations[serviceName][operation] = struct{}{}
	}
}

func (t *Tenant) storeTraces(traceId pcommon.TraceID, resourceSpanSlice ptrace.ResourceSpansSlice) {
	t.Lock()
	defer t.Unlock()
	if foundTraces, ok := t.traces[traceId]; !ok {
		traces := ptrace.NewTraces()
		resourceSpanSlice.MoveAndAppendTo(traces.ResourceSpans())
		t.traces[traceId] = traces
		// if we have a limit, let's cleanup the oldest traces
		if t.config.MaxTraces > 0 {
			// we only have to deal with this slice if we have a limit
			t.index = (t.index + 1) % t.config.MaxTraces
			// do we have an item already on this position? if so, we are overriding it,
			// and we need to remove from the map
			if !t.ids[t.index].IsEmpty() {
				delete(t.traces, t.ids[t.index])
			}
			// update the ring with the trace id
			t.ids[t.index] = traceId
		}
	} else {
		resourceSpanSlice.MoveAndAppendTo(foundTraces.ResourceSpans())
	}
}

// NewStore creates an unbounded in-memory store
func NewStore(cfg v1.Configuration) *Store {
	return &Store{
		defaultConfig: cfg,
		perTenant:     make(map[string]*Tenant),
	}
}

func newTenant(cfg *v1.Configuration) *Tenant {
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
			tenant = newTenant(&st.defaultConfig)
			st.perTenant[tenantID] = tenant
		}
	}
	return tenant
}

// WriteTraces write the traces into the tenant by grouping all the spans with same trace id together.
// The traces will not be saved as they are coming, rather they would be reshuffled.
func (st *Store) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	sameTraceIDResourceSpans := reshuffleTraces(td)
	m := st.getTenant(tenancy.GetTenant(ctx))
	for traceId, sameTraceIDResourceSpan := range sameTraceIDResourceSpans {
		for i := 0; i < sameTraceIDResourceSpan.Len(); i++ {
			resourceSpan := sameTraceIDResourceSpan.At(i)
			serviceName := getServiceNameFromResource(resourceSpan.Resource())
			m.storeService(serviceName)
			for j := 0; j < resourceSpan.ScopeSpans().Len(); j++ {
				scopeSpan := resourceSpan.ScopeSpans().At(j)
				for k := 0; k < scopeSpan.Spans().Len(); k++ {
					span := scopeSpan.Spans().At(k)
					operation := v2api.Operation{
						Name:     span.Name(),
						SpanKind: span.Kind().String(),
					}
					m.storeOperation(serviceName, operation)
				}
			}
		}
		m.storeTraces(traceId, sameTraceIDResourceSpan)
	}
	return nil
}

// reshuffleTraces reshuffles the traces so as to group the spans from same traces together. To understand this reshuffling
// take an example of traces which have 2 resource spans, then these two resource spans have 2 scope spans each.
// Every scope span consists of 2 spans with trace ids: 1 and 2. Now the final structure should look like:
// For TraceID1: [ResourceSpan1:[ScopeSpan1:[Span(TraceID1)],ScopeSpan2:[Span(TraceID1)], ResourceSpan2:[ScopeSpan1:[Span(TraceID1)],ScopeSpan2:[Span(TraceID1)]
// A similar structure will be there for TraceID2
func reshuffleTraces(td ptrace.Traces) map[pcommon.TraceID]ptrace.ResourceSpansSlice {
	sameTraceIDResourceSpans := make(map[pcommon.TraceID]ptrace.ResourceSpansSlice)
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		sameTraceIDScopeSpans := make(map[pcommon.TraceID]ptrace.ScopeSpansSlice)
		resourceSpan := td.ResourceSpans().At(i)
		reshuffleScopeSpan(resourceSpan, sameTraceIDScopeSpans)
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
	return sameTraceIDResourceSpans
}

// reshuffleScopeSpan reshuffles all the scope spans of a resource span to group
// spans of same trace ids together. The first step is to iterate the scope spans and then.
// copy the scope to an empty scopeSpan. After this, append the spans with same
// trace id to this empty scope span. Finally move this scope span to the scope span
// slice containing other scope spans and having same trace id.
func reshuffleScopeSpan(resourceSpan ptrace.ResourceSpans, sameTraceIDScopeSpans map[pcommon.TraceID]ptrace.ScopeSpansSlice) {
	for j := 0; j < resourceSpan.ScopeSpans().Len(); j++ {
		sameTraceIDSpans := make(map[pcommon.TraceID]ptrace.SpanSlice)
		scopeSpan := resourceSpan.ScopeSpans().At(j)
		reshuffleSpans(scopeSpan, sameTraceIDSpans)
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

func reshuffleSpans(scopeSpan ptrace.ScopeSpans, sameTraceIDSpans map[pcommon.TraceID]ptrace.SpanSlice) {
	for k := 0; k < scopeSpan.Spans().Len(); k++ {
		span := scopeSpan.Spans().At(k)
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
