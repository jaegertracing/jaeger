// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Tenant is an in-memory store of traces for a single tenant
type Tenant struct {
	sync.RWMutex
	ids        []pcommon.TraceID // ring buffer used to evict oldest traces
	traces     map[pcommon.TraceID]ptrace.Traces
	services   map[string]struct{}
	operations map[string]map[tracestore.Operation]struct{}
	config     *v1.Configuration
	evict      int // position in ids[] of the last evicted trace
}

func newTenant(cfg *v1.Configuration) *Tenant {
	return &Tenant{
		ids:        make([]pcommon.TraceID, cfg.MaxTraces),
		traces:     map[pcommon.TraceID]ptrace.Traces{},
		services:   map[string]struct{}{},
		operations: map[string]map[tracestore.Operation]struct{}{},
		config:     cfg,
		evict:      -1,
	}
}

func (t *Tenant) storeService(serviceName string) {
	t.Lock()
	defer t.Unlock()
	t.services[serviceName] = struct{}{}
}

func (t *Tenant) storeOperation(serviceName string, operation tracestore.Operation) {
	t.Lock()
	defer t.Unlock()
	if _, ok := t.operations[serviceName]; ok {
		t.operations[serviceName][operation] = struct{}{}
		return
	}
	t.operations[serviceName] = make(map[tracestore.Operation]struct{})
	t.operations[serviceName][operation] = struct{}{}
}

func (t *Tenant) storeTraces(traceId pcommon.TraceID, resourceSpanSlice ptrace.ResourceSpansSlice) {
	t.Lock()
	defer t.Unlock()
	if foundTraces, ok := t.traces[traceId]; ok {
		resourceSpanSlice.MoveAndAppendTo(foundTraces.ResourceSpans())
		return
	}
	traces := ptrace.NewTraces()
	resourceSpanSlice.MoveAndAppendTo(traces.ResourceSpans())
	t.traces[traceId] = traces
	// if we have a limit, let's cleanup the oldest traces
	if t.config.MaxTraces > 0 {
		// we only have to deal with this slice if we have a limit
		t.evict = (t.evict + 1) % t.config.MaxTraces
		// do we have an item already on this position? if so, we are overriding it,
		// and we need to remove from the map
		if !t.ids[t.evict].IsEmpty() {
			delete(t.traces, t.ids[t.evict])
		}
		// update the ring with the trace id
		t.ids[t.evict] = traceId
	}
}

func (t *Tenant) findTraces(query tracestore.TraceQueryParams) []ptrace.Traces {
	t.RLock()
	defer t.RUnlock()
	traces := make([]ptrace.Traces, 0, query.SearchDepth)
	lengthOfIds := len(t.ids)
	for index := range t.ids {
		traceId := t.ids[lengthOfIds-1-index]
		if !traceId.IsEmpty() {
			trace, ok := t.traces[traceId]
			if ok {
				if validTrace(trace, query) {
					if query.SearchDepth != 0 && len(traces) == query.SearchDepth {
						return traces
					}
					traces = append(traces, trace)
				}
			}
		}
	}
	return traces
}

func validTrace(td ptrace.Traces, query tracestore.TraceQueryParams) bool {
	for _, resourceSpan := range td.ResourceSpans().All() {
		if validResource(resourceSpan.Resource(), query) {
			for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
				for _, span := range scopeSpan.Spans().All() {
					if validSpan(resourceSpan.Resource().Attributes(), scopeSpan.Scope().Attributes(), span, query) {
						return true
					}
				}
			}
		}
	}
	return false
}

func validResource(resource pcommon.Resource, query tracestore.TraceQueryParams) bool {
	return query.ServiceName == "" || query.ServiceName == getServiceNameFromResource(resource)
}

func validSpan(resourceAttributes, scopeAttributes pcommon.Map, span ptrace.Span, query tracestore.TraceQueryParams) bool {
	errorAttributeFound := false
	if errorAttr, ok := query.Attributes.Get(errorAttribute); ok {
		errorAttributeFound = errorAttr.Bool()
		query.Attributes.Remove(errorAttribute)
	}
	if query.Attributes.Len() > 0 {
		for key, val := range query.Attributes.All() {
			if !findKeyValInTrace(key, val, resourceAttributes, scopeAttributes, span) {
				return false
			}
		}
	}
	if errorAttributeFound && span.Status().Code() != ptrace.StatusCodeError {
		return false
	}
	if query.OperationName != "" && query.OperationName != span.Name() {
		return false
	}
	startTime := span.StartTimestamp().AsTime()
	endTime := span.EndTimestamp().AsTime()
	if !query.StartTimeMin.IsZero() && startTime.Before(query.StartTimeMin) {
		return false
	}
	if !query.StartTimeMax.IsZero() && startTime.After(query.StartTimeMax) {
		return false
	}
	duration := endTime.Sub(span.StartTimestamp().AsTime())
	if query.DurationMin != 0 && duration < query.DurationMin {
		return false
	}
	if query.DurationMax != 0 && duration > query.DurationMax {
		return false
	}
	return true
}

func matchAttributes(key string, val pcommon.Value, attrs pcommon.Map) bool {
	if queryValue, ok := attrs.Get(key); ok {
		return queryValue.AsString() == val.AsString()
	}
	return false
}

func findKeyValInTrace(key string, val pcommon.Value, resourceAttributes pcommon.Map, scopeAttributes pcommon.Map, span ptrace.Span) bool {
	tagsMatched := matchAttributes(key, val, span.Attributes()) || matchAttributes(key, val, scopeAttributes) || matchAttributes(key, val, resourceAttributes)
	if !tagsMatched {
		for _, event := range span.Events().All() {
			if matchAttributes(key, val, event.Attributes()) {
				return true
			}
		}
	}
	return tagsMatched
}
