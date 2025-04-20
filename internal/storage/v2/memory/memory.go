// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"errors"
	"fmt"
	"github.com/jaegertracing/jaeger-idl/model/v1"
	"iter"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const (
	statusError      = "ERROR"
	statusOk         = "OK"
	tagW3CTraceState = "w3c.tracestate"
	tagHTTPStatusMsg = "http.status_message"
	tagError         = "error"
)

var errType = errors.New("invalid type")

// Store is an in-memory store of traces
type Store struct {
	sync.RWMutex
	// Each tenant gets a copy of default config.
	// In the future this can be extended to contain per-tenant configuration.
	defaultConfig v1.Configuration
	perTenant     map[string]*Tenant
}

// NewStore creates an in-memory store
func NewStore(cfg v1.Configuration) *Store {
	return &Store{
		defaultConfig: cfg,
		perTenant:     make(map[string]*Tenant),
	}
}

type memoryTraceQueryParams struct {
	ServiceName   string
	OperationName string
	Attributes    pcommon.Map
	StartTimeMin  time.Time
	StartTimeMax  time.Time
	DurationMin   time.Duration
	DurationMax   time.Duration
	SearchDepth   int
	Kind          ptrace.SpanKind
	Status        ptrace.Status
	TraceState    string
	ScopeName     string
	ScopeVersion  string
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
				}
			}
		}
		m.storeTraces(traceId, sameTraceIDResourceSpan)
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

type tracesByTime struct {
	ts     time.Time
	traces ptrace.Traces
}

func (st *Store) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		m := st.getTenant(tenancy.GetTenant(ctx))
		memTraceQueryParams := toMemoryTraceQueryParams(query)
		tracesByTm := make([]tracesByTime, 0)
		m.RLock()
		for _, trace := range m.traces {
			if startTime, ok := validTrace(trace, memTraceQueryParams); ok {
				tracesByTm = append(tracesByTm, tracesByTime{
					ts:     startTime,
					traces: trace,
				})
			}
		}
		// No need of lock now as result is fetched!
		m.RUnlock()
		// Query result order doesn't matter, as the query frontend will sort them anyway.
		// However, if query.NumTraces < results, then we should return the newest traces.
		if query.SearchDepth > 0 && len(tracesByTm) > query.SearchDepth {
			sort.Slice(tracesByTm, func(i, j int) bool {
				return tracesByTm[i].ts.Before(tracesByTm[j].ts)
			})
			tracesByTm = tracesByTm[len(tracesByTm)-query.SearchDepth:]
		}
		for _, trace := range tracesByTm {
			if !yield([]ptrace.Traces{trace.traces}, nil) {
				return
			}
		}
	}
}

func toMemoryTraceQueryParams(query tracestore.TraceQueryParams) memoryTraceQueryParams {
	attrs := pcommon.NewMap()
	query.Attributes.CopyTo(attrs)
	traceState := getTraceStateFromAttrs(attrs)
	kind := getSpanKindFromAttrs(attrs)
	status := getSpanStatusFromAttrs(attrs, kind)
	scopeName, scopeVersion := getScopeNameAndVersionFromAttrs(attrs)
	return memoryTraceQueryParams{
		OperationName: query.OperationName,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		SearchDepth:   query.SearchDepth,
		Attributes:    attrs,
		ServiceName:   query.ServiceName,
		TraceState:    traceState,
		Kind:          kind,
		Status:        status,
		ScopeName:     scopeName,
		ScopeVersion:  scopeVersion,
	}
}

func getScopeNameAndVersionFromAttrs(attrs pcommon.Map) (name string, version string) {
	if nameVal, ok := attrs.Get(conventions.AttributeOtelScopeName); ok {
		attrs.Remove(conventions.AttributeOtelScopeName)
		name = nameVal.Str()
	}
	if versionVal, ok := attrs.Get(conventions.AttributeOtelScopeVersion); ok {
		attrs.Remove(conventions.AttributeOtelScopeVersion)
		version = versionVal.Str()
	}
	return name, version
}

func getTraceStateFromAttrs(attrs pcommon.Map) string {
	traceState := ""
	// TODO Bring this inline with solution for jaegertracing/jaeger-client-java #702 once available
	if attr, ok := attrs.Get(tagW3CTraceState); ok {
		traceState = attr.Str()
		attrs.Remove(tagW3CTraceState)
	}
	return traceState
}

func getSpanKindFromAttrs(attrs pcommon.Map) ptrace.SpanKind {
	val, found := attrs.Get(model.SpanKindKey)
	if found {
		attrs.Remove(model.SpanKindKey)
		return stringToOTELSpanKind(val.Str())
	}
	return ptrace.SpanKindUnspecified
}

func stringToOTELSpanKind(spanKind string) ptrace.SpanKind {
	switch spanKind {
	case "client":
		return ptrace.SpanKindClient
	case "server":
		return ptrace.SpanKindServer
	case "producer":
		return ptrace.SpanKindProducer
	case "consumer":
		return ptrace.SpanKindConsumer
	case "internal":
		return ptrace.SpanKindInternal
	}
	return ptrace.SpanKindUnspecified
}

func getSpanStatusFromAttrs(attrs pcommon.Map, kind ptrace.SpanKind) ptrace.Status {
	dest := ptrace.NewStatus()
	statusCode := ptrace.StatusCodeUnset
	statusMessage := ""
	statusExists := false

	if errorVal, ok := attrs.Get(tagError); ok && errorVal.Type() == pcommon.ValueTypeBool {
		if errorVal.Bool() {
			statusCode = ptrace.StatusCodeError
			attrs.Remove(tagError)
			statusExists = true
			if desc, ok := extractStatusDescFromAttr(attrs); ok {
				statusMessage = desc
			} else if descAttr, ok := attrs.Get(tagHTTPStatusMsg); ok {
				statusMessage = descAttr.Str()
			}
		}
	}

	if codeAttr, ok := attrs.Get(conventions.OtelStatusCode); ok {
		if !statusExists {
			// The error tag is the ultimate truth for a Jaeger spans' error
			// status. Only parse the otel.status_code tag if the error tag is
			// not set to true.
			statusExists = true
			switch strings.ToUpper(codeAttr.Str()) {
			case statusOk:
				statusCode = ptrace.StatusCodeOk
			case statusError:
				statusCode = ptrace.StatusCodeError
			}

			if desc, ok := extractStatusDescFromAttr(attrs); ok {
				statusMessage = desc
			}
		}
		// Regardless of error tag inputValue, remove the otel.status_code tag. The
		// otel.status_message tag will have already been removed if
		// statusExists is true.
		attrs.Remove(conventions.OtelStatusCode)
	} else if httpCodeAttr, ok := attrs.Get(conventions.AttributeHTTPStatusCode); !statusExists && ok {
		// Fallback to introspecting if this span represents a failed HTTP
		// request or response, but again, only do so if the `error` tag was
		// not set to true and no explicit status was sent.
		if code, err := getStatusCodeFromHTTPStatusAttr(httpCodeAttr, kind); err == nil {
			if code != ptrace.StatusCodeUnset {
				statusExists = true
				statusCode = code
			}

			if msgAttr, ok := attrs.Get(tagHTTPStatusMsg); ok {
				statusMessage = msgAttr.Str()
			}
		}
	}

	if statusExists {
		dest.SetCode(statusCode)
		dest.SetMessage(statusMessage)
	}
	return dest
}

// extractStatusDescFromAttr returns the OTel status description from attrs
// along with true if it is set. Otherwise, an empty string and false are
// returned. The OTel status description attribute is deleted from attrs in
// the process.
func extractStatusDescFromAttr(attrs pcommon.Map) (string, bool) {
	if msgAttr, ok := attrs.Get(conventions.OtelStatusDescription); ok {
		msg := msgAttr.Str()
		attrs.Remove(conventions.OtelStatusDescription)
		return msg, true
	}
	return "", false
}

// codeFromAttr returns the integer code inputValue from attrVal. An error is
// returned if the code is not represented by an integer or string inputValue in
// the attrVal or the inputValue is outside the bounds of an int representation.
func codeFromAttr(attrVal pcommon.Value) (int64, error) {
	var val int64
	switch attrVal.Type() {
	case pcommon.ValueTypeInt:
		val = attrVal.Int()
	case pcommon.ValueTypeStr:
		var err error
		val, err = strconv.ParseInt(attrVal.Str(), 10, 0)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("%w: %s", errType, attrVal.Type().String())
	}
	return val, nil
}

func getStatusCodeFromHTTPStatusAttr(attrVal pcommon.Value, kind ptrace.SpanKind) (ptrace.StatusCode, error) {
	statusCode, err := codeFromAttr(attrVal)
	if err != nil {
		return ptrace.StatusCodeUnset, err
	}

	// For HTTP status codes in the 4xx range span status MUST be left unset
	// in case of SpanKind.SERVER and MUST be set to Error in case of SpanKind.CLIENT.
	// For HTTP status codes in the 5xx range, as well as any other code the client
	// failed to interpret, span status MUST be set to Error.
	if statusCode >= 400 && statusCode < 500 {
		switch kind {
		case ptrace.SpanKindClient:
			return ptrace.StatusCodeError, nil
		case ptrace.SpanKindServer:
			return ptrace.StatusCodeUnset, nil
		}
	}

	return statusCodeFromHTTP(statusCode), nil
}

// StatusCodeFromHTTP takes an HTTP status code and return the appropriate OpenTelemetry status code
// See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/http.md#status
func statusCodeFromHTTP(httpStatusCode int64) ptrace.StatusCode {
	if httpStatusCode >= 100 && httpStatusCode < 399 {
		return ptrace.StatusCodeUnset
	}
	return ptrace.StatusCodeError
}

func validTrace(td ptrace.Traces, query memoryTraceQueryParams) (time.Time, bool) {
	for _, resourceSpan := range td.ResourceSpans().All() {
		tags := make([]keyValue, 0)
		if validResource(resourceSpan.Resource(), query) {
			tags = insertTagsFromAttrs(tags, resourceSpan.Resource().Attributes())
			for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
				if validScope(scopeSpan.Scope(), query) {
					tags = insertTagsFromAttrs(tags, scopeSpan.Scope().Attributes())
					for _, span := range scopeSpan.Spans().All() {
						if validSpan(tags, span, query) {
							return span.StartTimestamp().AsTime(), true
						}
					}
				}
			}
		}
	}
	return time.Time{}, false
}

func validResource(resource pcommon.Resource, query memoryTraceQueryParams) bool {
	if query.ServiceName != getServiceNameFromResource(resource) {
		return false
	}
	return true
}

func validScope(scope pcommon.InstrumentationScope, query memoryTraceQueryParams) bool {
	if query.ScopeName != "" && query.ScopeName != scope.Name() {
		return false
	}
	if query.ScopeVersion != "" && query.ScopeVersion != scope.Version() {
		return false
	}
	return true
}

type keyValue struct {
	key   string
	value pcommon.Value
}

func validSpan(tags []keyValue, span ptrace.Span, query memoryTraceQueryParams) bool {
	tags = insertTagsFromAttrs(tags, span.Attributes())
	for _, val := range span.Events().All() {
		tags = insertTagsFromAttrs(tags, val.Attributes())
	}
	for queryK, queryV := range query.Attributes.All() {
		if ok := findKeyValueMatch(tags, queryK, queryV); !ok {
			return false
		}
	}
	if query.Kind != ptrace.SpanKindUnspecified && query.Kind != span.Kind() {
		return false
	}
	if query.OperationName != span.Name() {
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
	if query.TraceState != "" && query.TraceState != span.TraceState().AsRaw() {
		return false
	}
	return true
}

func findKeyValueMatch(kvs []keyValue, key string, value pcommon.Value) bool {
	for _, kv := range kvs {
		if kv.key == key && kv.value.Equal(value) {
			return true
		}
	}
	return false
}

func insertTagsFromAttrs(kvs []keyValue, attrs pcommon.Map) []keyValue {
	for key, val := range attrs.All() {
		kvs = append(kvs, keyValue{
			key:   key,
			value: val,
		})
	}
	return kvs
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
