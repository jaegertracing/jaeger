// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
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

// matchedSpan is a matching span with its resource and scope.
type matchedSpan struct {
	key          sortingKey
	resourceSpan ptrace.ResourceSpans
	scopeSpan    ptrace.ScopeSpans
	span         ptrace.Span
}

// matchedTrace is a matching trace with its sort key.
type matchedTrace struct {
	key   traceKey
	entry traceAndId
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
		t.mostRecent = (t.mostRecent + 1) % len(t.traces)
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

// findTraceAndIds returns references to the traces the store holds, not copies,
// so that FindTraceIDs pays nothing for trace data it discards. Anything that
// hands the traces to a reader must pass them through cloneTrace first.
func (t *Tenant) findTraceAndIds(query tracestore.TraceQueryParams) ([]traceAndId, error) {
	if query.SearchDepth == 0 || query.SearchDepth > t.config.MaxTraces {
		return nil, errInvalidSearchDepth
	}
	filter, err := prepareFilter(query.Filter)
	if err != nil {
		return nil, err
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	traceAndIds := make([]traceAndId, 0, query.SearchDepth)
	n := len(t.traces)
	for i := range t.traces {
		if uint64(len(traceAndIds)) == uint64(query.SearchDepth) {
			break
		}
		index := (t.mostRecent - i + n) % n
		traceById := t.traces[index]
		if traceById.id.IsEmpty() {
			// Finding an empty ID means we reached a gap in the ring buffer
			// that has not yet been filled with traces.
			break
		}
		if validTrace(traceById.trace, query, filter) {
			traceAndIds = append(traceAndIds, traceById)
		}
	}
	return traceAndIds, nil
}

// findSpans returns one page of the spans, across all traces, that start
// within [query.StartTimeMin, query.StartTimeMax] (a zero bound is unbounded)
// and match query.Filter. Matches are sorted by sortingKey; the page starts after
// the cursor `after` (at the beginning when nil) and holds at most
// query.Pagination.PageSize spans when that is positive. A zero PageSize
// returns every match: SpanQueryParams carries Pagination by value, so a zero
// page size is the absence of a request rather than the malformed one it is
// on a trace query, and the query service always sets one (RFC 0016 §6). The
// returned cursor ends the page if more matches remain, and is nil otherwise.
// Each span is copied with its own resource and scope, since one result can
// hold spans from many traces and resources (RFC 0016), so the result shares
// nothing with the store.
//
// query.Filter is prepared once, before any span is visited, rather than per
// span: its shape and its regular expressions are static properties of the
// filter, not something that can vary span to span.
func (t *Tenant) findSpans(query tracestore.SpanQueryParams, after *cursor[sortingKey]) (ptrace.Traces, *cursor[sortingKey], error) {
	filter, err := prepareFilter(query.Filter)
	if err != nil {
		return ptrace.Traces{}, nil, err
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	var matches []matchedSpan
	for i := range t.traces {
		entry := t.traces[i]
		if entry.id.IsEmpty() {
			continue
		}
		for _, resourceSpan := range entry.trace.ResourceSpans().All() {
			for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
				for _, span := range scopeSpan.Spans().All() {
					if !spanStartsWithin(span, query.StartTimeMin, query.StartTimeMax) {
						continue
					}
					if !matchesFilter(filter, resourceSpan.Resource(), scopeSpan.Scope(), span,
						resourceSpan.SchemaUrl(), scopeSpan.SchemaUrl()) {
						continue
					}
					ctx := filterCtx{
						resource:          resourceSpan.Resource(),
						scope:             scopeSpan.Scope(),
						span:              span,
						resourceSchemaURL: resourceSpan.SchemaUrl(),
						scopeSchemaURL:    scopeSpan.SchemaUrl(),
					}
					matches = append(matches, matchedSpan{
						key:          makeSortingKey(ctx, query.OrderBy),
						resourceSpan: resourceSpan,
						scopeSpan:    scopeSpan,
						span:         span,
					})
				}
			}
		}
	}
	// The sort is stable so that copies of one span, which share a key, keep the order they
	// were written in across searches, which the cursor's count of returned copies relies on.
	compare := compareSortingKeys(query.OrderBy)
	slices.SortStableFunc(matches, func(a, b matchedSpan) int { return compare(a.key, b.key) })
	matches, last := page(matches, func(m matchedSpan) sortingKey { return m.key }, compare, after, query.Pagination.PageSize)
	result := ptrace.NewTraces()
	for _, m := range matches {
		rs := result.ResourceSpans().AppendEmpty()
		m.resourceSpan.Resource().CopyTo(rs.Resource())
		rs.SetSchemaUrl(m.resourceSpan.SchemaUrl())
		ss := rs.ScopeSpans().AppendEmpty()
		m.scopeSpan.Scope().CopyTo(ss.Scope())
		ss.SetSchemaUrl(m.scopeSpan.SchemaUrl())
		m.span.CopyTo(ss.Spans().AppendEmpty())
	}
	// CopyTo assembles the page but shares bytes-valued attributes with the store (see
	// cloneTrace), so the assembled page is copied a second time, deeply, before a caller
	// may modify it.
	result, err = cloneTrace(result)
	if err != nil {
		return ptrace.Traces{}, nil, err
	}
	return result, last, nil
}

// findTraceAndIdsPage is findTraceAndIds for a query with Pagination: the
// matching traces sorted by traceKey, starting after the cursor `after` (at
// the beginning when nil), at most PageSize of them. The returned cursor ends
// the page if more matches remain, and is nil otherwise. Like findTraceAndIds
// it returns references, not copies.
func (t *Tenant) findTraceAndIdsPage(query tracestore.TraceQueryParams, after *cursor[traceKey]) ([]traceAndId, *cursor[traceKey], error) {
	if query.Pagination.PageSize <= 0 {
		return nil, nil, fmt.Errorf("%w: page size must be greater than 0", tracestore.ErrPaginationInvalid)
	}
	filter, err := prepareFilter(query.Filter)
	if err != nil {
		return nil, nil, err
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	var matches []matchedTrace
	for i := range t.traces {
		entry := t.traces[i]
		if entry.id.IsEmpty() {
			continue
		}
		startTime, ok := latestMatchingSpanStart(entry.trace, query, filter)
		if !ok {
			continue
		}
		matches = append(matches, matchedTrace{key: traceKey{startTime: startTime, traceID: entry.id}, entry: entry})
	}
	slices.SortStableFunc(matches, func(a, b matchedTrace) int { return compareTraceKeys(a.key, b.key) })
	matches, last := page(matches, func(m matchedTrace) traceKey { return m.key }, compareTraceKeys, after, query.Pagination.PageSize)
	traceAndIds := make([]traceAndId, len(matches))
	for i, m := range matches {
		traceAndIds[i] = m.entry
	}
	return traceAndIds, last, nil
}

func spanStartsWithin(span ptrace.Span, startTimeMin, startTimeMax time.Time) bool {
	startTime := span.StartTimestamp().AsTime()
	if !startTimeMin.IsZero() && startTime.Before(startTimeMin) {
		return false
	}
	if !startTimeMax.IsZero() && startTime.After(startTimeMax) {
		return false
	}
	return true
}

// getTraces returns references to the traces the store holds, not copies; see
// findTraceAndIds. Callers must clone before handing them to a reader.
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
					if !found || parentSpanServiceName == spanServiceName {
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

// validTrace reports whether any span of the trace matches the query.
func validTrace(td ptrace.Traces, query tracestore.TraceQueryParams, filter preparedFilter) bool {
	matched := false
	forEachMatchingSpan(td, query, filter, func(ptrace.Span) bool {
		matched = true
		return false
	})
	return matched
}

// latestMatchingSpanStart returns the latest start time among the trace's
// matching spans, and false if none matches.
func latestMatchingSpanStart(td ptrace.Traces, query tracestore.TraceQueryParams, filter preparedFilter) (pcommon.Timestamp, bool) {
	var latest pcommon.Timestamp
	matched := false
	forEachMatchingSpan(td, query, filter, func(span ptrace.Span) bool {
		matched = true
		latest = max(latest, span.StartTimestamp())
		return true
	})
	return latest, matched
}

// forEachMatchingSpan calls visit on each span of the trace that matches the
// query, stopping when visit returns false.
func forEachMatchingSpan(td ptrace.Traces, query tracestore.TraceQueryParams, filter preparedFilter, visit func(ptrace.Span) bool) {
	for _, resourceSpan := range td.ResourceSpans().All() {
		// query.ServiceName is always empty when query.Filter is set (the two are
		// mutually exclusive, enforced before a Reader ever sees the query), so this
		// resource pre-check is a no-op for a filter query rather than something
		// validSpan's filter branch needs to duplicate.
		if !validResource(resourceSpan.Resource(), query) {
			continue
		}
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if validSpan(resourceSpan.Resource(), scopeSpan.Scope(), span, query, filter,
					resourceSpan.SchemaUrl(), scopeSpan.SchemaUrl()) && !visit(span) {
					return
				}
			}
		}
	}
}

func validResource(resource pcommon.Resource, query tracestore.TraceQueryParams) bool {
	return query.ServiceName == "" || query.ServiceName == getServiceNameFromResource(resource)
}

func validSpan(
	resource pcommon.Resource, scope pcommon.InstrumentationScope, span ptrace.Span, query tracestore.TraceQueryParams,
	filter preparedFilter, resourceSchemaURL, scopeSchemaURL string,
) bool {
	if query.Filter != nil {
		// The structured filter is a complete alternative to every predicate field
		// below it (ServiceName, OperationName, Attributes, the duration bounds), not
		// one more thing to combine with them (enforced by EnsureFilterStandsAlone
		// before a Reader ever sees the query), so it fully replaces this function's
		// legacy path rather than adding to it. The time range stays a query-level
		// bound on FindTraces regardless of which predicate model is in play.
		return spanStartsWithin(span, query.StartTimeMin, query.StartTimeMax) &&
			matchesFilter(filter, resource, scope, span, resourceSchemaURL, scopeSchemaURL)
	}

	resourceAttributes := resource.Attributes()
	if query.OperationName != "" && query.OperationName != span.Name() {
		return false
	}

	if !spanStartsWithin(span, query.StartTimeMin, query.StartTimeMax) {
		return false
	}
	duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
	if query.DurationMin != 0 && duration < query.DurationMin {
		return false
	}
	if query.DurationMax != 0 && duration > query.DurationMax {
		return false
	}

	if errAttribute, ok := query.Attributes.Get(errorAttribute); ok {
		errorVal, valid := errorQueryValue(errAttribute)
		if !valid {
			return false
		}
		// error=true matches only Error spans; error=false is its complement — every
		// span that is not an error, i.e. Ok *and* the default Unset status. Requiring
		// Ok here would drop Unset spans, which are the common case, so error=false
		// would return almost nothing.
		if errorVal && span.Status().Code() != ptrace.StatusCodeError {
			return false
		}
		if !errorVal && span.Status().Code() == ptrace.StatusCodeError {
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

func errorQueryValue(attr pcommon.Value) (value bool, valid bool) {
	switch attr.Type() {
	case pcommon.ValueTypeBool:
		return attr.Bool(), true
	case pcommon.ValueTypeStr:
		val, err := strconv.ParseBool(attr.Str())
		if err != nil {
			return false, false
		}
		return val, true
	default:
		return false, false
	}
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
