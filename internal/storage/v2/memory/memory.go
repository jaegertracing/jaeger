// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const errorAttribute = "error"

var errInvalidSearchDepth = errors.New("search depth must be greater than 0 and less than max traces")

// Store is an in-memory store of traces
type Store struct {
	sync.RWMutex
	// Each tenant gets a copy of default config.
	// In the future this can be extended to contain per-tenant configuration.
	defaultConfig v1.Configuration
	perTenant     map[string]*Tenant
}

// NewStore creates an in-memory store
func NewStore(cfg v1.Configuration) (*Store, error) {
	if cfg.MaxTraces <= 0 {
		return nil, errInvalidMaxTraces
	}
	return &Store{
		defaultConfig: cfg,
		perTenant:     make(map[string]*Tenant),
	}, nil
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
	resourceSpansByTraceId := reshuffleResourceSpans(td.ResourceSpans())
	m := st.getTenant(tenancy.GetTenant(ctx))
	for traceId, sameTraceIDResourceSpan := range resourceSpansByTraceId {
		var startTime time.Time
		var endTime time.Time
		for _, resourceSpan := range sameTraceIDResourceSpan.All() {
			serviceName := getServiceNameFromResource(resourceSpan.Resource())
			if serviceName == "" {
				continue
			}
			m.storeService(serviceName)
			for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
				for _, span := range scopeSpan.Spans().All() {
					operation := tracestore.Operation{
						Name:     span.Name(),
						SpanKind: span.Kind().String(),
					}
					m.storeOperation(serviceName, operation)
					if startTime.IsZero() || endTime.IsZero() {
						startTime = span.StartTimestamp().AsTime()
						endTime = span.EndTimestamp().AsTime()
					} else {
						spanStartTime := span.StartTimestamp().AsTime()
						if spanStartTime.Before(startTime) {
							startTime = spanStartTime
						}
						spanEndTime := span.EndTimestamp().AsTime()
						if spanEndTime.After(endTime) {
							endTime = spanEndTime
						}
					}
				}
			}
		}
		m.storeTraces(traceId, startTime, endTime, sameTraceIDResourceSpan)
	}
	return nil
}

// GetOperations returns operations based on the service name and span kind
func (st *Store) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.RLock()
	defer m.RUnlock()
	var retMe []tracestore.Operation
	if operations, ok := m.operations[query.ServiceName]; ok {
		for operation := range operations {
			if query.SpanKind == "" || query.SpanKind == operation.SpanKind {
				retMe = append(retMe, operation)
			}
		}
	}
	return retMe, nil
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

func (st *Store) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return func(yield func([]ptrace.Traces, error) bool) {
		traceAndIds, err := m.findTraceAndIds(query)
		if err != nil {
			yield(nil, err)
			return
		}
		for i := range traceAndIds {
			if !yield([]ptrace.Traces{traceAndIds[i].trace}, nil) {
				return
			}
		}
	}
}

func (st *Store) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		traceAndIds, err := m.findTraceAndIds(query)
		if err != nil {
			yield(nil, err)
			return
		}
		ids := make([]tracestore.FoundTraceID, len(traceAndIds))
		for i := range traceAndIds {
			ids[i] = tracestore.FoundTraceID{TraceID: traceAndIds[i].id}
		}
		yield(ids, nil)
	}
}

func (st *Store) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return func(yield func([]ptrace.Traces, error) bool) {
		traces := m.getTraces(traceIDs...)
		for i := range traces {
			if !yield([]ptrace.Traces{traces[i]}, nil) {
				return
			}
		}
	}
}

func (st *Store) GetDependencies(ctx context.Context, query depstore.QueryParameters) ([]model.DependencyLink, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return m.getDependencies(query)
}

// reshuffleResourceSpans reshuffles the resource spans so as to group the spans from same traces together. To understand this reshuffling
// take an example of 2 resource spans, then these two resource spans have 2 scope spans each.
// Every scope span consists of 2 spans with trace ids: 1 and 2. Now the final structure should look like:
// For TraceID1: [ResourceSpan1:[ScopeSpan1:[Span(TraceID1)],ScopeSpan2:[Span(TraceID1)], ResourceSpan2:[ScopeSpan1:[Span(TraceID1)],ScopeSpan2:[Span(TraceID1)]
// A similar structure will be there for TraceID2
func reshuffleResourceSpans(resourceSpanSlice ptrace.ResourceSpansSlice) map[pcommon.TraceID]ptrace.ResourceSpansSlice {
	resourceSpansByTraceId := make(map[pcommon.TraceID]ptrace.ResourceSpansSlice)
	for _, resourceSpan := range resourceSpanSlice.All() {
		scopeSpansByTraceId := reshuffleScopeSpans(resourceSpan.ScopeSpans())
		// All the  scope spans here will have the same resource as of resourceSpan. Therefore:
		// Copy the resource to an empty resourceSpan. After this, append the scope spans with same
		// trace id to this empty resource span. Finally move this resource span to the resourceSpanSlice
		// containing other resource spans and having same trace id.
		for traceId, scopeSpansSlice := range scopeSpansByTraceId {
			resourceSpanByTraceId := ptrace.NewResourceSpans()
			resourceSpan.Resource().CopyTo(resourceSpanByTraceId.Resource())
			scopeSpansSlice.MoveAndAppendTo(resourceSpanByTraceId.ScopeSpans())
			resourceSpansSlice, ok := resourceSpansByTraceId[traceId]
			if !ok {
				resourceSpansSlice = ptrace.NewResourceSpansSlice()
				resourceSpansByTraceId[traceId] = resourceSpansSlice
			}
			resourceSpanByTraceId.MoveTo(resourceSpansSlice.AppendEmpty())
		}
	}
	return resourceSpansByTraceId
}

// reshuffleScopeSpans reshuffles all the scope spans of a resource span to group
// spans of same trace ids together. The first step is to iterate the scope spans and then.
// copy the scope to an empty scopeSpan. After this, append the spans with same
// trace id to this empty scope span. Finally move this scope span to the scope span
// slice containing other scope spans and having same trace id.
func reshuffleScopeSpans(scopeSpanSlice ptrace.ScopeSpansSlice) map[pcommon.TraceID]ptrace.ScopeSpansSlice {
	scopeSpansByTraceId := make(map[pcommon.TraceID]ptrace.ScopeSpansSlice)
	for _, scopeSpan := range scopeSpanSlice.All() {
		spansByTraceId := reshuffleSpans(scopeSpan.Spans())
		for traceId, spansSlice := range spansByTraceId {
			scopeSpanByTraceId := ptrace.NewScopeSpans()
			scopeSpan.Scope().CopyTo(scopeSpanByTraceId.Scope())
			spansSlice.MoveAndAppendTo(scopeSpanByTraceId.Spans())
			scopeSpansSlice, ok := scopeSpansByTraceId[traceId]
			if !ok {
				scopeSpansSlice = ptrace.NewScopeSpansSlice()
				scopeSpansByTraceId[traceId] = scopeSpansSlice
			}
			scopeSpanByTraceId.MoveTo(scopeSpansSlice.AppendEmpty())
		}
	}
	return scopeSpansByTraceId
}

func reshuffleSpans(spanSlice ptrace.SpanSlice) map[pcommon.TraceID]ptrace.SpanSlice {
	spansByTraceId := make(map[pcommon.TraceID]ptrace.SpanSlice)
	for _, span := range spanSlice.All() {
		spansSlice, ok := spansByTraceId[span.TraceID()]
		if !ok {
			spansSlice = ptrace.NewSpanSlice()
			spansByTraceId[span.TraceID()] = spansSlice
		}
		span.CopyTo(spansSlice.AppendEmpty())
	}
	return spansByTraceId
}

func getServiceNameFromResource(resource pcommon.Resource) string {
	val, ok := resource.Attributes().Get(conventions.AttributeServiceName)
	if !ok {
		return ""
	}
	return val.Str()
}
