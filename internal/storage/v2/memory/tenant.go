// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"errors"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var errInvalidMaxTraces = errors.New("max traces must be greater than zero")

// Tenant is an in-memory store of traces for a single tenant
type Tenant struct {
	mu     sync.RWMutex
	config *Configuration

	ids        map[pcommon.TraceID]int // maps trace id to index in traces[]
	traces     []traceAndId            // ring buffer to store traces
	mostRecent int                     // position in traces[] of the most recently added trace

	services   map[string]struct{}
	operations map[string]map[tracestore.Operation]struct{}
}

type traceAndId struct {
	id        pcommon.TraceID
	trace     ptrace.Traces
	startTime time.Time
	endTime   time.Time
}

func (t traceAndId) traceIsBetweenStartAndEnd(startTime time.Time, endTime time.Time) bool {
	if endTime.IsZero() {
		return t.startTime.After(startTime)
	}
	return t.startTime.After(startTime) && t.endTime.Before(endTime)
}

func newTenant(cfg *Configuration) *Tenant {
	return &Tenant{
		config:     cfg,
		ids:        make(map[pcommon.TraceID]int),
		traces:     make([]traceAndId, cfg.MaxTraces),
		mostRecent: -1,
		services:   map[string]struct{}{},
		operations: map[string]map[tracestore.Operation]struct{}{},
	}
}

func (t *Tenant) storeTraces(tracesById map[pcommon.TraceID]ptrace.ResourceSpansSlice) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for traceId, sameTraceIDResourceSpan := range tracesById {
		var startTime time.Time
		var endTime time.Time
		for _, resourceSpan := range sameTraceIDResourceSpan.All() {
			serviceName := getServiceNameFromResource(resourceSpan.Resource())
			if serviceName != "" {
				t.services[serviceName] = struct{}{}
			}
			for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
				for _, span := range scopeSpan.Spans().All() {
					if serviceName != "" {
						operation := tracestore.Operation{
							Name:     span.Name(),
							SpanKind: fromOTELSpanKind(span.Kind()),
						}
						if _, ok := t.operations[serviceName]; !ok {
							t.operations[serviceName] = make(map[tracestore.Operation]struct{})
						}
						t.operations[serviceName][operation] = struct{}{}
					}
					if startTime.IsZero() || span.StartTimestamp().AsTime().Before(startTime) {
						startTime = span.StartTimestamp().AsTime()
					}
					if endTime.IsZero() || span.EndTimestamp().AsTime().After(endTime) {
						endTime = span.EndTimestamp().AsTime()
					}
				}
			}
		}
		if index, ok := t.ids[traceId]; ok {
			sameTraceIDResourceSpan.MoveAndAppendTo(t.traces[index].trace.ResourceSpans())
			if startTime.Before(t.traces[index].startTime) {
				t.traces[index].startTime = startTime
			}
			if endTime.After(t.traces[index].endTime) {
				t.traces[index].endTime = endTime
			}
			continue
		}
		traces := ptrace.NewTraces()
		sameTraceIDResourceSpan.MoveAndAppendTo(traces.ResourceSpans())
		t.mostRecent = (t.mostRecent + 1) % t.config.MaxTraces
		// if there is already a trace in lastEvicted position, remove its ID from ids map
		if !t.traces[t.mostRecent].id.IsEmpty() {
			delete(t.ids, t.traces[t.mostRecent].id)
		}
		// update the ring with the trace id
		t.ids[traceId] = t.mostRecent
		t.traces[t.mostRecent] = traceAndId{
			id:        traceId,
			trace:     traces,
			startTime: startTime,
			endTime:   endTime,
		}
	}
}

func (t *Tenant) findTraceAndIds(query tracestore.TraceQueryParams) ([]traceAndId, error) {
	if query.SearchDepth <= 0 || query.SearchDepth > t.config.MaxTraces {
		return nil, errInvalidSearchDepth
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	traceAndIds := make([]traceAndId, 0, query.SearchDepth)
	n := len(t.traces)
	for i := range t.traces {
		if len(traceAndIds) == query.SearchDepth {
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
			traceAndIds = append(traceAndIds, traceById)
		}
	}
	return traceAndIds, nil
}

func (t *Tenant) getTraces(traceIds ...tracestore.GetTraceParams) []ptrace.Traces {
	t.mu.RLock()
	defer t.mu.RUnlock()
	traces := make([]ptrace.Traces, 0)
	for i := range traceIds {
		index, ok := t.ids[traceIds[i].TraceID]
		if ok {
			traces = append(traces, t.traces[index].trace)
		}
	}
	return traces
}

func (t *Tenant) getDependencies(query depstore.QueryParameters) ([]model.DependencyLink, error) {
	if query.StartTime.IsZero() {
		return nil, errors.New("start time is required")
	}
	if !query.EndTime.IsZero() && query.EndTime.Before(query.StartTime) {
		return nil, errors.New("end time must be greater than start time")
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	deps := map[string]*model.DependencyLink{}
	for _, index := range t.ids {
		traceWithTime := t.traces[index]
		if !traceWithTime.traceIsBetweenStartAndEnd(query.StartTime, query.EndTime) {
			continue
		}
		for _, resourceSpan := range traceWithTime.trace.ResourceSpans().All() {
			for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
				for _, span := range scopeSpan.Spans().All() {
					if span.ParentSpanID().IsEmpty() {
						continue
					}
					spanServiceName := getServiceNameFromResource(resourceSpan.Resource())
					parentSpanServiceName, found := findServiceNameWithSpanId(traceWithTime.trace, span.ParentSpanID())
					if !found {
						continue
					}
					depKey := parentSpanServiceName + "&&&" + spanServiceName
					if _, ok := deps[depKey]; !ok {
						deps[depKey] = &model.DependencyLink{
							Parent:    parentSpanServiceName,
							Child:     spanServiceName,
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

func findServiceNameWithSpanId(trace ptrace.Traces, spanId pcommon.SpanID) (string, bool) {
	for _, resourceSpan := range trace.ResourceSpans().All() {
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if span.SpanID() == spanId {
					return getServiceNameFromResource(resourceSpan.Resource()), true
				}
			}
		}
	}
	return "", false
}

func validTrace(td ptrace.Traces, query tracestore.TraceQueryParams) bool {
	for _, resourceSpan := range td.ResourceSpans().All() {
		if !validResource(resourceSpan.Resource(), query) {
			continue
		}
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if validSpan(resourceSpan.Resource().Attributes(), scopeSpan.Scope(), span, query) {
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

func validSpan(resourceAttributes pcommon.Map, scope pcommon.InstrumentationScope, span ptrace.Span, query tracestore.TraceQueryParams) bool {
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

	if errAttribute, ok := query.Attributes.Get(errorAttribute); ok {
		if errAttribute.Bool() && span.Status().Code() != ptrace.StatusCodeError {
			return false
		}
		if !errAttribute.Bool() && span.Status().Code() != ptrace.StatusCodeOk {
			return false
		}
	}

	if statusAttr, ok := query.Attributes.Get("span.status"); ok {
		expectedStatus := spanStatusFromString(statusAttr.AsString())
		if expectedStatus != span.Status().Code() {
			return false
		}
	}

	if kindAttr, ok := query.Attributes.Get("span.kind"); ok {
		expectedKind := spanKindFromString(kindAttr.AsString())
		if expectedKind != span.Kind() {
			return false
		}
	}

	if scopeNameAttr, ok := query.Attributes.Get("scope.name"); ok {
		if scopeNameAttr.AsString() != scope.Name() {
			return false
		}
	}

	if scopeVersionAttr, ok := query.Attributes.Get("scope.version"); ok {
		if scopeVersionAttr.AsString() != scope.Version() {
			return false
		}
	}

	for key, val := range query.Attributes.All() {
		if key == errorAttribute ||
			key == "span.status" ||
			key == "span.kind" ||
			key == "scope.name" ||
			key == "scope.version" {
			continue
		}

		if resourceKey, ok := strings.CutPrefix(key, "resource."); ok {
			if !matchAttributes(resourceKey, val, resourceAttributes) {
				return false
			}
			continue
		}

		if !findKeyValInTrace(key, val, resourceAttributes, scope.Attributes(), span) {
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
	for _, link := range span.Links().All() {
		if matchAttributes(key, val, link.Attributes()) {
			return true
		}
	}
	return tagsMatched
}

func fromOTELSpanKind(kind ptrace.SpanKind) string {
	if kind == ptrace.SpanKindUnspecified {
		return ""
	}
	return strings.ToLower(kind.String())
}

func spanStatusFromString(statusStr string) ptrace.StatusCode {
	switch strings.ToUpper(statusStr) {
	case "OK":
		return ptrace.StatusCodeOk
	case "ERROR":
		return ptrace.StatusCodeError
	default:
		return ptrace.StatusCodeUnset
	}
}

func spanKindFromString(kindStr string) ptrace.SpanKind {
	switch strings.ToUpper(kindStr) {
	case "CLIENT":
		return ptrace.SpanKindClient
	case "SERVER":
		return ptrace.SpanKindServer
	case "PRODUCER":
		return ptrace.SpanKindProducer
	case "CONSUMER":
		return ptrace.SpanKindConsumer
	case "INTERNAL":
		return ptrace.SpanKindInternal
	default:
		return ptrace.SpanKindUnspecified
	}
}
