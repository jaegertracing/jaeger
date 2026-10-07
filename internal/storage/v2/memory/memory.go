// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const errorAttribute = "error"

var errInvalidSearchDepth = errors.New("search depth must be greater than 0 and less than max traces")

// Store is an in-memory store of traces
type Store struct {
	// The in-memory store does not compute trace summaries natively; fall back to
	// FindTraces + client-side aggregation.
	tracestore.UnsupportedTraceSummaries

	mu sync.RWMutex
	// Each tenant gets a copy of default config.
	// In the future this can be extended to contain per-tenant configuration.
	cfg       Configuration
	perTenant map[string]*Tenant
}

// NewStore creates an in-memory store
func NewStore(cfg Configuration) (*Store, error) {
	if cfg.MaxTraces == 0 {
		return nil, errInvalidMaxTraces
	}
	return &Store{
		cfg:       cfg,
		perTenant: make(map[string]*Tenant),
	}, nil
}

// getTenant returns the per-tenant storage.  Note that tenantID has already been checked for by the collector or query
func (st *Store) getTenant(tenantID string) *Tenant {
	st.mu.RLock()
	tenant, ok := st.perTenant[tenantID]
	st.mu.RUnlock()
	if !ok {
		st.mu.Lock()
		defer st.mu.Unlock()
		tenant, ok = st.perTenant[tenantID]
		if !ok {
			tenant = newTenant(&st.cfg)
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
	m.storeTraces(resourceSpansByTraceId)
	return nil
}

// GetOperations returns operations based on the service name and span kind
func (st *Store) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	var retMe []string
	for k := range m.services {
		retMe = append(retMe, k)
	}
	return retMe, nil
}

func (*Store) SearchCapabilities(context.Context) (tracestore.SearchCapabilities, error) {
	return tracestore.SearchCapabilities{
		// The span matcher treats an empty query service name as "match any", so an
		// omitted name spans every service in the store.
		WithoutServiceName: true,
		// The reference store evaluates every level and operator the filter AST
		// defines (see filter.go), so it declares the full vocabulary rather than a
		// subset the way a real backend limited by its indexing would, less the
		// operators it has not learned yet.
		Filter: &tracestore.FilterCapabilities{
			Levels:    expression.Levels(),
			Operators: supportedOperators(),
		},
		// FindSpans below evaluates the same filter engine as FindTraces, over
		// every span in the store rather than per matched trace (RFC 0016).
		SpanSearch:  true,
		SpanSorting: true,
		// FindTraceIDs and FindSpans sort their results and page through them with a
		// keyset cursor (pagination.go).
		Paginated: true,
	}, nil
}

// FindSpans returns every span in the store matching query, across however
// many traces they belong to (RFC 0016). Unlike FindTraces, which finds
// traces containing at least one matching span, FindSpans's result holds
// exactly the matching spans and nothing else: two matching spans that
// happen to share a trace do not pull in the rest of that trace's spans, and
// a matching span always keeps its own resource and scope, not its trace's
// other spans' resources.
//
// The result is one page, sorted by sortingKey and bounded by
// query.Pagination.PageSize when that is positive. The chunk carries the next
// page's token if more spans match (RFC 0014).
func (st *Store) FindSpans(ctx context.Context, query tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		// The query service settles the order before calling a reader, but a reader reached
		// directly must still refuse terms it cannot execute, and the settled order is the one
		// the fingerprint and the sort must agree on.
		order, err := tracestore.EffectiveSpanOrder(query.OrderBy)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		query.OrderBy = order
		fingerprint, err := query.Fingerprint()
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		after, err := cursorOf(query.Pagination.PageToken, fingerprint, func(raw []byte) (cursor[sortingKey], error) {
			return decodeSpanCursor(raw, len(query.OrderBy))
		})
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		matched, last, err := m.findSpans(query, after)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		chunk := tracestore.PageChunk[ptrace.Traces]{Results: matched}
		if last != nil {
			if chunk.NextPageToken, err = tracestore.NewPageToken(fingerprint, last.encode()); err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
		}
		yield(chunk, nil)
	}
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
			trace, err := cloneTrace(traceAndIds[i].trace)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield([]ptrace.Traces{trace}, nil) {
				return
			}
		}
	}
}

// FindTraceIDs without Pagination returns the most recently written matching
// traces up to SearchDepth, as FindTraces does. With Pagination it sorts the
// matching traces by traceKey, returns the page that follows the query's
// token, at most PageSize traces, and carries the next page's token if more
// traces match.
func (st *Store) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[tracestore.PageChunk[[]tracestore.FoundTraceID], error] {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return func(yield func(tracestore.PageChunk[[]tracestore.FoundTraceID], error) bool) {
		var chunk tracestore.PageChunk[[]tracestore.FoundTraceID]
		fail := func(err error) {
			yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{}, err)
		}
		var traceAndIds []traceAndId
		if query.Pagination == nil {
			var err error
			if traceAndIds, err = m.findTraceAndIds(query); err != nil {
				fail(err)
				return
			}
		} else {
			fingerprint, err := query.Fingerprint()
			if err != nil {
				fail(err)
				return
			}
			after, err := cursorOf(query.Pagination.PageToken, fingerprint, decodeTraceCursor)
			if err != nil {
				fail(err)
				return
			}
			var last *cursor[traceKey]
			if traceAndIds, last, err = m.findTraceAndIdsPage(query, after); err != nil {
				fail(err)
				return
			}
			if last != nil {
				if chunk.NextPageToken, err = tracestore.NewPageToken(fingerprint, last.encode()); err != nil {
					fail(err)
					return
				}
			}
		}
		chunk.Results = make([]tracestore.FoundTraceID, len(traceAndIds))
		for i := range traceAndIds {
			chunk.Results[i] = tracestore.FoundTraceID{TraceID: traceAndIds[i].id}
		}
		yield(chunk, nil)
	}
}

func (st *Store) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return func(yield func([]ptrace.Traces, error) bool) {
		traces := m.getTraces(traceIDs...)
		for i := range traces {
			trace, err := cloneTrace(traces[i])
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield([]ptrace.Traces{trace}, nil) {
				return
			}
		}
	}
}

// cloneTrace deep-copies a stored trace before it is handed to a reader.
//
// The Tenant accessors return the ptrace.Traces the store holds, and pdata
// handles are references into shared backing storage, so without this copy a
// reader that modifies what it received would rewrite the stored trace for
// every later reader. Query-time adjusters do exactly that — the adjuster
// interface is documented as modifying a trace in place — as does any
// query-interceptor extension that redacts attributes on the return path.
//
// This restores for the v2 store the guarantee #2720 added to the v1 store,
// which was lost when the v1 implementation was deleted in #7711, and it uses
// the same mechanism. A proto round-trip is the only copy that is deep for every
// field: ptrace.Traces.CopyTo looks like the cheaper equivalent, but it assigns
// bytes-valued attributes by slice (`ov.BytesValue = t.BytesValue` in pdata's
// generated CopyAnyValue), leaving the clone sharing a backing array with the
// store, which a reader can then rewrite through pcommon.ByteSlice.SetAt.
// Nested array and kvlist values are copied properly, so bytes are the only
// gap — but relying on that is how this class of bug survives, and the contract
// this upholds promises callers a trace they may modify however they like.
func cloneTrace(src ptrace.Traces) (ptrace.Traces, error) {
	buf, err := marshalTraces(src)
	if err != nil {
		return ptrace.Traces{}, fmt.Errorf("cannot copy stored trace: %w", err)
	}
	dst, err := unmarshalTraces(buf)
	if err != nil {
		return ptrace.Traces{}, fmt.Errorf("cannot copy stored trace: %w", err)
	}
	return dst, nil
}

// Indirected so that tests can exercise cloneTrace's error paths, which pdata
// does not produce for a well-formed trace held by the store.
var (
	marshalTraces   = new(ptrace.ProtoMarshaler).MarshalTraces
	unmarshalTraces = new(ptrace.ProtoUnmarshaler).UnmarshalTraces
)

func (st *Store) GetDependencies(ctx context.Context, query depstore.QueryParameters) ([]model.DependencyLink, error) {
	m := st.getTenant(tenancy.GetTenant(ctx))
	return m.getDependencies(query)
}

func (st *Store) Purge() error {
	st.mu.Lock()
	st.perTenant = make(map[string]*Tenant)
	st.mu.Unlock()
	return nil
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
			resourceSpanByTraceId.SetSchemaUrl(resourceSpan.SchemaUrl())
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
			scopeSpanByTraceId.SetSchemaUrl(scopeSpan.SchemaUrl())
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
	val, ok := resource.Attributes().Get(conventions.ServiceNameKey)
	if !ok {
		return ""
	}
	return val.Str()
}

// unsupportedOperators lists the operators that the vocabulary defines and that filter.go does
// not evaluate yet. A new operator reaches this store through a jaeger-idl release before the
// store learns it, and the store must not declare one it would refuse (RFC 0005 §9, M8).
var unsupportedOperators = []expression.Operator{expression.OpPhrase, expression.OpFulltext}

// supportedOperators returns the vocabulary minus the unsupportedOperators list.
func supportedOperators() []expression.Operator {
	return slices.DeleteFunc(expression.Operators(), func(op expression.Operator) bool {
		return slices.Contains(unsupportedOperators, op)
	})
}
