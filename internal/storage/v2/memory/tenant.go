// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"errors"
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var errInvalidMaxTraces = errors.New("max traces must be greater than zero")

// Tenant is an in-memory store of traces for a single tenant
type Tenant struct {
	sync.RWMutex
	config *v1.Configuration

	ids        map[pcommon.TraceID]int // maps trace id to index in traces[]
	traces     []traceAndId            // ring buffer to store traces
	mostRecent int                     // position in traces[] of the most recently added trace

	services   map[string]struct{}
	operations map[string]map[tracestore.Operation]struct{}
}

type traceAndId struct {
	id    pcommon.TraceID
	trace ptrace.Traces
}

func newTenant(cfg *v1.Configuration) *Tenant {
	return &Tenant{
		config:     cfg,
		ids:        make(map[pcommon.TraceID]int),
		traces:     make([]traceAndId, cfg.MaxTraces),
		mostRecent: -1,
		services:   map[string]struct{}{},
		operations: map[string]map[tracestore.Operation]struct{}{},
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
	if index, ok := t.ids[traceId]; ok {
		resourceSpanSlice.MoveAndAppendTo(t.traces[index].trace.ResourceSpans())
		return
	}
	traces := ptrace.NewTraces()
	resourceSpanSlice.MoveAndAppendTo(traces.ResourceSpans())
	t.mostRecent = (t.mostRecent + 1) % t.config.MaxTraces
	// if there is already a trace in lastEvicted position, remove its ID from ids map
	if !t.traces[t.mostRecent].id.IsEmpty() {
		delete(t.ids, t.traces[t.mostRecent].id)
	}
	// update the ring with the trace id
	t.ids[traceId] = t.mostRecent
	t.traces[t.mostRecent] = traceAndId{
		id:    traceId,
		trace: traces,
	}
}

func (t *Tenant) findTraces(query tracestore.TraceQueryParams) []ptrace.Traces {
	t.RLock()
	defer t.RUnlock()
	traces := make([]ptrace.Traces, 0, query.SearchDepth)
	n := len(t.traces)
	for i := range t.traces {
		if len(traces) == query.SearchDepth {
			break
		}
		index := (t.mostRecent - i + n) % n
		traceById := t.traces[index]
		if traceById.id.IsEmpty() {
			// Finding an empty ID means we reached a gap in the ring buffer
			// that has not yet been filled with traces.
			break
		}
		if validTrace(traceById.trace, query) {
			traces = append(traces, traceById.trace)
		}
	}
	return traces
}

func validTrace(td ptrace.Traces, query tracestore.TraceQueryParams) bool {
	for _, resourceSpan := range td.ResourceSpans().All() {
		if !validResource(resourceSpan.Resource(), query) {
			continue
		}
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if validSpan(resourceSpan.Resource().Attributes(), scopeSpan.Scope().Attributes(), span, query) {
					return true
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
	if errAttribute, ok := query.Attributes.Get(errorAttribute); ok {
		if errAttribute.Bool() && span.Status().Code() != ptrace.StatusCodeError {
			return false
		}
		if !errAttribute.Bool() && span.Status().Code() != ptrace.StatusCodeOk {
			return false
		}
	}
	if query.OperationName != "" && query.OperationName != span.Name() {
		return false
	}
	startTime := span.StartTimestamp().AsTime()
	if !query.StartTimeMin.IsZero() && startTime.Before(query.StartTimeMin) {
		return false
	}
	if !query.StartTimeMax.IsZero() && startTime.After(query.StartTimeMax) {
		return false
	}
	duration := span.EndTimestamp().AsTime().Sub(startTime)
	if query.DurationMin != 0 && duration < query.DurationMin {
		return false
	}
	if query.DurationMax != 0 && duration > query.DurationMax {
		return false
	}
	for key, val := range query.Attributes.All() {
		if key != errorAttribute && !findKeyValInTrace(key, val, resourceAttributes, scopeAttributes, span) {
			return false
		}
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
	if tagsMatched {
		return true
	}
	for _, event := range span.Events().All() {
		if matchAttributes(key, val, event.Attributes()) {
			return true
		}
	}
	return tagsMatched
}
