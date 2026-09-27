// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// fakeReader is a minimal tracestore.Reader that records the query and the context it received
// and yields a single configured batch (or error).
type fakeReader struct {
	gotQuery        tracestore.TraceQueryParams
	gotSpanQuery    tracestore.SpanQueryParams
	gotCtx          context.Context
	findCalled      bool
	batch           []ptrace.Traces
	err             error
	leadingErr      error
	summaryCalled   bool
	gotSummaryQuery tracestore.TraceQueryParams
	summaries       []tracestore.TraceSummary
	summaryErr      error
	capabilities    *tracestore.SearchCapabilities
	capabilityReads int
	nextPageToken   string
}

// SearchCapabilities answers for a backend that searches every service and evaluates no filter
// unless a test says otherwise, since that is the shape most of these searches assume.
func (f *fakeReader) SearchCapabilities(context.Context) (tracestore.SearchCapabilities, error) {
	f.capabilityReads++
	if f.capabilities == nil {
		return tracestore.SearchCapabilities{WithoutServiceName: true, SpanSearch: true}, nil
	}
	return *f.capabilities, nil
}

func (f *fakeReader) FindTraces(ctx context.Context, q tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	f.findCalled = true
	f.gotQuery = q
	f.gotCtx = ctx
	return func(yield func([]ptrace.Traces, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(f.batch, nil)
	}
}

func (f *fakeReader) GetTraces(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		if f.leadingErr != nil {
			if !yield(nil, f.leadingErr) {
				return
			}
		}
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(f.batch, nil)
	}
}

func (*fakeReader) FindTraceIDs(context.Context, tracestore.TraceQueryParams) iter.Seq2[tracestore.PageChunk[[]tracestore.FoundTraceID], error] {
	return func(func(tracestore.PageChunk[[]tracestore.FoundTraceID], error) bool) {}
}

func (f *fakeReader) FindSpans(ctx context.Context, q tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	f.findCalled = true
	f.gotSpanQuery = q
	f.gotCtx = ctx
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		if f.leadingErr != nil {
			if !yield(tracestore.PageChunk[ptrace.Traces]{}, f.leadingErr) {
				return
			}
		}
		if f.err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, f.err)
			return
		}
		for i, spans := range f.batch {
			chunk := tracestore.PageChunk[ptrace.Traces]{Results: spans}
			if i == len(f.batch)-1 {
				chunk.NextPageToken = tracestore.PageToken(f.nextPageToken)
			}
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

func (*fakeReader) GetServices(context.Context) ([]string, error) {
	return []string{"svc"}, nil
}

func (*fakeReader) GetOperations(context.Context, tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return []tracestore.Operation{{Name: "op"}}, nil
}

func (f *fakeReader) FindTraceSummaries(_ context.Context, q tracestore.TraceQueryParams) iter.Seq2[tracestore.PageChunk[[]tracestore.TraceSummary], error] {
	f.summaryCalled = true
	f.gotSummaryQuery = q
	return func(yield func(tracestore.PageChunk[[]tracestore.TraceSummary], error) bool) {
		if f.summaryErr != nil {
			yield(tracestore.PageChunk[[]tracestore.TraceSummary]{}, f.summaryErr)
			return
		}
		yield(tracestore.PageChunk[[]tracestore.TraceSummary]{Results: f.summaries}, nil)
	}
}

// multiBatchReader yields a fixed sequence of trace batches, so a test can assert what the query
// service does with batches after the first.
type multiBatchReader struct {
	*fakeReader
	batches [][]ptrace.Traces
}

func (r *multiBatchReader) FindTraces(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return r.yieldBatches
}

func (r *multiBatchReader) FindSpans(context.Context, tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return r.yieldSpanBatches
}

func (r *multiBatchReader) GetTraces(context.Context, ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return r.yieldBatches
}

func (r *multiBatchReader) yieldBatches(yield func([]ptrace.Traces, error) bool) {
	for _, b := range r.batches {
		if !yield(b, nil) {
			return
		}
	}
}

func (r *multiBatchReader) yieldSpanBatches(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
	for _, b := range r.batches {
		for _, spans := range b {
			chunk := tracestore.PageChunk[ptrace.Traces]{Results: spans}
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

// fakeInterceptor lets each test supply the hook behavior it needs. It receives the interceptor's
// TraceQuery, exactly as a real interceptor would. The optional onQueryCtx and onResultCtx hooks
// transform (and observe) the context, so a test can assert how the query service threads it from
// a pre-query hook into the reader and the matching result hook. The same context hooks serve
// both the trace and the span pair.
type fakeInterceptor struct {
	onQuery      func(queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error)
	onResult     func([]ptrace.Traces) ([]ptrace.Traces, error)
	onSpanQuery  func(queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error)
	onSpanResult func(ptrace.Traces) (ptrace.Traces, error)
	onQueryCtx   func(context.Context) context.Context
	onResultCtx  func(context.Context) context.Context
}

func (f fakeInterceptor) OnTraceQuery(ctx context.Context, q queryinterceptor.TraceQuery) (context.Context, queryinterceptor.TraceQuery, error) {
	if f.onQueryCtx != nil {
		ctx = f.onQueryCtx(ctx)
	}
	if f.onQuery != nil {
		nq, err := f.onQuery(q)
		return ctx, nq, err
	}
	return ctx, q, nil
}

func (f fakeInterceptor) OnTraceResult(ctx context.Context, t []ptrace.Traces) (context.Context, []ptrace.Traces, error) {
	if f.onResultCtx != nil {
		ctx = f.onResultCtx(ctx)
	}
	if f.onResult != nil {
		nt, err := f.onResult(t)
		return ctx, nt, err
	}
	return ctx, t, nil
}

func (f fakeInterceptor) OnSpanQuery(ctx context.Context, q queryinterceptor.SpanQuery) (context.Context, queryinterceptor.SpanQuery, error) {
	if f.onQueryCtx != nil {
		ctx = f.onQueryCtx(ctx)
	}
	if f.onSpanQuery != nil {
		nq, err := f.onSpanQuery(q)
		return ctx, nq, err
	}
	return ctx, q, nil
}

func (f fakeInterceptor) OnSpanResult(ctx context.Context, spans ptrace.Traces) (context.Context, ptrace.Traces, error) {
	if f.onResultCtx != nil {
		ctx = f.onResultCtx(ctx)
	}
	if f.onSpanResult != nil {
		ns, err := f.onSpanResult(spans)
		return ctx, ns, err
	}
	return ctx, spans, nil
}

// interceptedService builds a query service that runs the given interceptors over next.
func interceptedService(next tracestore.Reader, interceptors ...queryinterceptor.Interceptor) *QueryService {
	return NewQueryService(next, nil, QueryServiceOptions{Interceptors: interceptors})
}

// searchQuery asks for raw traces, so that the batches a test asserts on are the ones the reader
// yielded and the interceptor rewrote, rather than the aggregated traces built from them.
// searchQuery wraps a reader query for a test about something other than its envelope, so it
// fills in the time range every search must carry unless the test set one itself.
func searchQuery(q TraceQueryParams) TraceQueryParams {
	if q.StartTimeMin.IsZero() && q.StartTimeMax.IsZero() {
		q.StartTimeMin, q.StartTimeMax = testWindowStart, testWindowEnd
	}
	q.RawTraces = true
	return q
}

// searchSpansQuery wraps a reader query for a test about something other than its envelope, so
// it fills in the time range every search must carry unless the test set one itself.
func searchSpansQuery(q SpanQueryParams) SpanQueryParams {
	if q.StartTimeMin.IsZero() && q.StartTimeMax.IsZero() {
		q.StartTimeMin, q.StartTimeMax = testWindowStart, testWindowEnd
	}
	return q
}

// serviceFilter builds the predicate `resource.service == name`, which is how an access-control
// interceptor scopes a search to a service the caller may read.
func serviceFilter(name string) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.FieldRef{Level: expression.LevelResource, Name: expression.ResourceFieldService},
		&expression.StringValue{Value: name},
	}}
}

// routeFilter builds the predicate `span.http.route == "/cart"`, a predicate qualified by the
// level it applies to.
func routeFilter() *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
		&expression.AnyValue{Value: "/cart"},
	}}
}

func narrowSpansTo(filter *expression.Call) func(queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
	return func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
		q.Filter = filter
		return q, nil
	}
}

func narrowTo(filter *expression.Call) func(queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
	return func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
		q.Filter = filter
		return q, nil
	}
}

func tracesWith(key, val string) []ptrace.Traces {
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr(key, val)
	return []ptrace.Traces{td}
}

func firstSpanAttr(t *testing.T, batch []ptrace.Traces, key string) string {
	t.Helper()
	require.NotEmpty(t, batch)
	attrs := batch[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	v, ok := attrs.Get(key)
	require.True(t, ok, "attribute %q not present", key)
	return v.Str()
}

func collectTraces(it iter.Seq2[[]ptrace.Traces, error]) ([][]ptrace.Traces, error) {
	var out [][]ptrace.Traces
	for batch, err := range it {
		if err != nil {
			return out, err
		}
		out = append(out, batch)
	}
	return out, nil
}

// redactSpanResult is redactResult for one chunk of spans.
func redactSpanResult(key string) func(ptrace.Traces) (ptrace.Traces, error) {
	redact := redactResult(key)
	return func(spans ptrace.Traces) (ptrace.Traces, error) {
		out, err := redact([]ptrace.Traces{spans})
		if err != nil {
			return spans, err
		}
		return out[0], nil
	}
}

func redactResult(key string) func([]ptrace.Traces) ([]ptrace.Traces, error) {
	return func(batch []ptrace.Traces) ([]ptrace.Traces, error) {
		for _, td := range batch {
			rss := td.ResourceSpans()
			for i := 0; i < rss.Len(); i++ {
				sss := rss.At(i).ScopeSpans()
				for j := 0; j < sss.Len(); j++ {
					spans := sss.At(j).Spans()
					for k := 0; k < spans.Len(); k++ {
						if _, ok := spans.At(k).Attributes().Get(key); ok {
							spans.At(k).Attributes().PutStr(key, "REDACTED")
						}
					}
				}
			}
		}
		return batch, nil
	}
}

func attributesWith(key, val string) pcommon.Map {
	m := pcommon.NewMap()
	m.PutStr(key, val)
	return m
}

func TestFindTraces_AppliesQueryAndResultHooks(t *testing.T) {
	next := &fakeReader{batch: tracesWith("secret", "value")}
	qs := interceptedService(next, fakeInterceptor{
		onQuery:  narrowTo(serviceFilter("gated")),
		onResult: redactResult("secret"),
	})

	out, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
		ServiceName: "original",
	})))
	require.NoError(t, err)
	assert.Equal(t, "gated", next.gotQuery.ServiceName, "pre-query hook must reach storage")
	require.Len(t, out, 1)
	assert.Equal(t, "REDACTED", firstSpanAttr(t, out[0], "secret"), "result hook must redact")
}

// TestFindTraces_ShowsEveryPredicateAsAFilter pins what an interceptor is shown and what storage
// receives afterwards. Whatever shape a query arrives in, the interceptor sees every predicate in
// the filter; what reaches storage is the shape the backend declared it evaluates, chosen once
// after the interceptor has had its say.
func TestFindTraces_ShowsEveryPredicateAsAFilter(t *testing.T) {
	t.Run("a scalar query is shown as a filter", func(t *testing.T) {
		var seen queryinterceptor.TraceQuery
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
				seen = q
				q.Filter = serviceFilter("gated")
				return q, nil
			},
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			ServiceName: "original",
			Attributes:  attributesWith("http.method", "GET"),
		})))
		require.NoError(t, err)

		// The interceptor saw predicates, not fields.
		require.NotNil(t, seen.Filter)
		assert.Equal(t, expression.OpAnd, seen.Filter.Op)
		assert.Len(t, seen.Filter.Args, 2, "the service and the tag both became predicates")

		// This backend evaluates no filter, so it got fields again, carrying what the
		// interceptor chose.
		assert.Equal(t, "gated", next.gotQuery.ServiceName)
		assert.Nil(t, next.gotQuery.Filter)
	})

	// The caller's filter is what the interceptor is shown, so a predicate qualified by the level
	// it applies to still names that level. An access-control interceptor keys on the level, and
	// would otherwise judge a predicate the caller never sent.
	t.Run("a caller's filter keeps the level it named", func(t *testing.T) {
		enableStructuredFilters(t)
		sent := routeFilter()
		var seen queryinterceptor.TraceQuery
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
				seen = q
				return q, nil
			},
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			Filter: sent,
		})))
		require.NoError(t, err)
		assert.Equal(t, sent, seen.Filter, "the caller's filter reaches the interceptor verbatim")
		// The same query still reaches this filter-less backend as the tag search it can serve.
		value, ok := next.gotQuery.Attributes.Get("http.route")
		require.True(t, ok)
		assert.Equal(t, "/cart", value.Str())
	})

	t.Run("a filter reaches a backend that evaluates one", func(t *testing.T) {
		enableStructuredFilters(t)
		filter := routeFilter()
		next := &fakeReader{batch: tracesWith("k", "v")}
		next.capabilities = &tracestore.SearchCapabilities{
			WithoutServiceName: true,
			Filter: &tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpEq},
			},
		}
		qs := interceptedService(next, fakeInterceptor{})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			Filter: filter,
		})))
		require.NoError(t, err)
		assert.Equal(t, filter, next.gotQuery.Filter, "a reader that evaluates filters keeps one")
		assert.Empty(t, next.gotQuery.ServiceName)
	})
}

// TestFindTraces_RefusesAPredicateTheBackendCannotServe pins that the capability check applies to
// the interceptor's output and not only to what the caller sent: the query service converts once,
// after OnTraceQuery, so a predicate an interceptor added is refused on the same terms as the caller's
// own.
func TestFindTraces_RefusesAPredicateTheBackendCannotServe(t *testing.T) {
	t.Run("the backend evaluates no filter and the fields cannot carry it", func(t *testing.T) {
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: narrowTo(&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
				serviceFilter("a"), serviceFilter("b"),
			}}),
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			ServiceName: "original",
		})))
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
		assert.False(t, next.findCalled, "storage must not be queried")
	})

	t.Run("the backend evaluates filters but not this level", func(t *testing.T) {
		enableStructuredFilters(t)
		spanPredicate := routeFilter()
		next := &fakeReader{batch: tracesWith("k", "v")}
		next.capabilities = &tracestore.SearchCapabilities{
			WithoutServiceName: true,
			Filter: &tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpAnd, expression.OpEq},
			},
		}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: narrowTo(&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				spanPredicate, serviceFilter("gated"),
			}}),
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			Filter: spanPredicate,
		})))
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
		require.ErrorContains(t, err, `does not index the "resource" level`)
		assert.False(t, next.findCalled, "storage must not be queried")
	})
}

// TestFindTraces_RefusesAnInvalidInterceptorFilter covers what an interceptor can return that
// storage must not see. Nothing here is the caller's fault, so none of it reads as a bad request,
// and none of it reaches storage — which would answer the malformed trees by matching nothing and
// the missing one by matching everything, silently undoing the narrowing an access-control
// interceptor exists to apply.
func TestFindTraces_RefusesAnInvalidInterceptorFilter(t *testing.T) {
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name:        "no filter at all, for a query that had predicates",
			expectedErr: "widen the search to everything in the time range",
		},
		{
			name:        "a conjunction of one",
			filter:      &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{serviceFilter("gated")}},
			expectedErr: `operator "and" takes at least two arguments`,
		},
		{
			name: "an operator this build does not define",
			filter: &expression.Call{Op: "matches", Args: []expression.Expression{
				&expression.AttributeRef{Key: "a"}, &expression.AnyValue{Value: "b"},
			}},
			expectedErr: `unknown filter operator "matches"`,
		},
		{
			name: "a comparison missing an operand",
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.AnyValue{Value: "a"},
			}},
			expectedErr: `operator "eq" takes`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next := &fakeReader{batch: tracesWith("k", "v")}
			qs := interceptedService(next, fakeInterceptor{onQuery: narrowTo(test.filter)})

			_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
				ServiceName: "original",
			})))
			require.ErrorIs(t, err, ErrInterceptorFilter)
			require.ErrorContains(t, err, test.expectedErr)
			assert.False(t, next.findCalled, "storage must not be queried")
			assert.False(t, IsBadRequest(err), "the caller's request was fine")
		})
	}
}

// filterCapableBackend declares a backend that serves the span level and the comparisons these
// tests build, so a test about the tree reaching storage is not also a test of what a backend
// declared.
func filterCapableBackend() *tracestore.SearchCapabilities {
	return &tracestore.SearchCapabilities{
		WithoutServiceName: true,
		SpanSearch:         true,
		Filter: &tracestore.FilterCapabilities{
			Levels: []expression.Level{expression.LevelSpan, expression.LevelResource},
			Operators: []expression.Operator{
				expression.OpEq, expression.OpGt, expression.OpLt, expression.OpIn,
			},
		},
	}
}

// TestFindTraces_LeavesALegacyQueryAloneWhenNothingChangedIt pins that enabling an interceptor does
// not move a result set by itself. A legacy query is shown to the interceptor as a filter, and an
// interceptor that returns it unchanged has said nothing about how it should reach storage — so the
// legacy query is dispatched as it arrived. It matters because a backend may search a legacy
// attribute more widely than an unqualified filter reference: Elasticsearch reads the legacy tag
// search over the event location too, while the filter's unqualified default is span-or-resource.
func TestFindTraces_LeavesALegacyQueryAloneWhenNothingChangedIt(t *testing.T) {
	t.Run("an interceptor that changes nothing", func(t *testing.T) {
		enableStructuredFilters(t)
		next := &fakeReader{batch: tracesWith("k", "v")}
		next.capabilities = filterCapableBackend()
		qs := interceptedService(next, fakeInterceptor{})

		attributes := pcommon.NewMap()
		attributes.PutStr("http.route", "/cart")
		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			ServiceName: "cart",
			Attributes:  attributes,
		})))
		require.NoError(t, err)
		assert.Nil(t, next.gotQuery.Filter, "storage was given the legacy query it would have had")
		assert.Equal(t, "cart", next.gotQuery.ServiceName)
		assert.Equal(t, "/cart", next.gotQuery.Attributes.AsRaw()["http.route"])
	})

	t.Run("an interceptor that narrows the filter", func(t *testing.T) {
		enableStructuredFilters(t)
		next := &fakeReader{batch: tracesWith("k", "v")}
		next.capabilities = filterCapableBackend()
		narrowed := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
			&expression.StringValue{Value: "gated"},
		}}
		qs := interceptedService(next, fakeInterceptor{onQuery: narrowTo(narrowed)})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			ServiceName: "cart",
		})))
		require.NoError(t, err)
		assert.Equal(t, narrowed, next.gotQuery.Filter, "the interceptor's filter is what storage gets")
	})

	t.Run("an interceptor that changes only the envelope", func(t *testing.T) {
		enableStructuredFilters(t)
		narrowedEnd := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
		next := &fakeReader{batch: tracesWith("k", "v")}
		next.capabilities = filterCapableBackend()
		qs := interceptedService(next, fakeInterceptor{
			onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
				q.StartTimeMax = narrowedEnd
				return q, nil
			},
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			ServiceName: "cart",
			SearchDepth: 7,
		})))
		require.NoError(t, err)
		assert.Nil(t, next.gotQuery.Filter, "the predicates are still the legacy ones")
		assert.Equal(t, "cart", next.gotQuery.ServiceName)
		assert.Equal(t, narrowedEnd, next.gotQuery.StartTimeMax, "the envelope change survives")
		assert.Equal(t, 7, next.gotQuery.SearchDepth, "the result bound is not the interceptor's to change")
	})

	t.Run("a caller's own filter is unaffected by the rule", func(t *testing.T) {
		enableStructuredFilters(t)
		next := &fakeReader{batch: tracesWith("k", "v")}
		next.capabilities = filterCapableBackend()
		qs := interceptedService(next, fakeInterceptor{})

		filter := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
			&expression.AnyValue{Value: "cart"},
		}}
		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
			Filter: filter,
		})))
		require.NoError(t, err)
		require.NotNil(t, next.gotQuery.Filter, "a filter the caller sent stays a filter")
	})
}

// TestFindTraces_RefusesAFilterALaterInterceptorDrops pins that the nil rule holds across the
// chain, not only between the query's first and last shape. A legacy query with no predicates
// gains a restriction from the first interceptor; the second returns no filter. Comparing only the
// ends would see nil on both and send the query to storage in its original, unrestricted shape.
func TestFindTraces_RefusesAFilterALaterInterceptorDrops(t *testing.T) {
	enableStructuredFilters(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = filterCapableBackend()
	qs := interceptedService(next,
		fakeInterceptor{onQuery: narrowTo(serviceFilter("gated"))},
		fakeInterceptor{onQuery: narrowTo(nil)},
	)

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.ErrorIs(t, err, ErrInterceptorFilter)
	require.ErrorContains(t, err, "widen the search")
	assert.False(t, next.findCalled, "storage must not be queried")
	assert.False(t, IsBadRequest(err), "the caller's request was fine")
}

// TestFindTraces_FinalizesAnInterceptorFilter pins that a predicate an interceptor adds reaches
// storage in the same shape as one a caller sent: its constants read against the fields they are
// compared to, and its comparisons turned so the reference comes first. Only checking the structure
// would hand a backend an unread string where it expects a length of time.
func TestFindTraces_FinalizesAnInterceptorFilter(t *testing.T) {
	tests := []struct {
		name     string
		returned *expression.Call
		expected *expression.Call
	}{
		{
			name: "a duration compared against the field that holds one",
			returned: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "2s"},
			}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name: "an instant compared against a timestamp field",
			returned: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "2026-08-18T00:00:00Z"},
			}},
			expected: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.TimestampValue{Value: time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)},
			}},
		},
		{
			name: "a comparison written with the constant on the left",
			returned: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.AnyValue{Value: "2s"},
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
			}},
			expected: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name: "a typed list against an attribute",
			returned: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
			expected: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
		},
		{
			name: "a word one of the closed sets holds",
			returned: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "server"},
			}},
			expected: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
				&expression.StringValue{Value: "server"},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enableStructuredFilters(t)
			next := &fakeReader{batch: tracesWith("k", "v")}
			next.capabilities = filterCapableBackend()
			qs := interceptedService(next, fakeInterceptor{onQuery: narrowTo(test.returned)})

			_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
				ServiceName: "original",
			})))
			require.NoError(t, err)
			assert.Equal(t, test.expected, next.gotQuery.Filter)
		})
	}
}

// TestFindTraces_RefusesAnInterceptorConstantThatWillNotParse is the other half: finalizing an
// interceptor's filter can refuse it for the same reason a caller's is refused, and the caller is
// told the interceptor was at fault rather than blamed for its own request.
func TestFindTraces_RefusesAnInterceptorConstantThatWillNotParse(t *testing.T) {
	enableStructuredFilters(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = filterCapableBackend()
	returned := &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
		&expression.AnyValue{Value: "banana"},
	}}
	qs := interceptedService(next, fakeInterceptor{onQuery: narrowTo(returned)})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
		ServiceName: "original",
	})))
	require.ErrorIs(t, err, ErrInterceptorFilter)
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)
	assert.False(t, next.findCalled, "storage must not be queried")
	assert.False(t, IsBadRequest(err), "the caller's request was fine")
}

// TestFindTraces_AllowsNoFilterForAPredicatelessQuery is the other side of the nil rule: a search
// of the time range alone has no filter to begin with, so an interceptor that leaves it that way
// has widened nothing and the search proceeds.
func TestFindTraces_AllowsNoFilterForAPredicatelessQuery(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v")}
	var seen queryinterceptor.TraceQuery
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
			seen = q
			return q, nil
		},
	})

	out, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Nil(t, seen.Filter, "there were no predicates to show")
	assert.True(t, next.findCalled)
}

func TestFindTraces_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) { return q, sentinel },
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.ErrorIs(t, err, sentinel)
	assert.False(t, next.findCalled, "storage must not be queried when the query is rejected")
}

func TestFindTraces_ResultErrorAborts(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, sentinel },
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.ErrorIs(t, err, sentinel)
}

func TestGetTraces_AppliesResultHook(t *testing.T) {
	next := &fakeReader{batch: tracesWith("secret", "value")}
	qs := interceptedService(next, fakeInterceptor{onResult: redactResult("secret")})

	out, err := collectTraces(qs.GetTraces(t.Context(), GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{}},
		RawTraces: true,
	}))
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "REDACTED", firstSpanAttr(t, out[0], "secret"))
}

func TestGetTraces_ResultErrorAborts(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, sentinel },
	})
	_, err := collectTraces(qs.GetTraces(t.Context(), GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{}},
		RawTraces: true,
	}))
	require.ErrorIs(t, err, sentinel)
}

func TestGetTraces_ContinuesAfterError(t *testing.T) {
	sentinel := errors.New("transient")
	next := &fakeReader{leadingErr: sentinel, batch: tracesWith("secret", "value")}
	qs := interceptedService(next, fakeInterceptor{onResult: redactResult("secret")})

	var errs, batches int
	for batch, err := range qs.GetTraces(t.Context(), GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{}},
		RawTraces: true,
	}) {
		if err != nil {
			require.ErrorIs(t, err, sentinel)
			errs++
			continue
		}
		assert.Equal(t, "REDACTED", firstSpanAttr(t, batch, "secret"))
		batches++
	}
	assert.Equal(t, 1, errs)
	assert.Equal(t, 1, batches)
}

// assertResultErrorStops verifies that an OnTraceResult failure aborts the stream: even a consumer
// that keeps ranging after the error must never receive a later batch, and OnTraceResult must not run
// again. This guards the redaction/authorization use case, where emitting a later batch after a
// failed sanitize would leak data.
func assertResultErrorStops(t *testing.T, call func(*QueryService) iter.Seq2[[]ptrace.Traces, error]) {
	sentinel := errors.New("sanitize failed")
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	onResultCalls := 0
	qs := interceptedService(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) {
			onResultCalls++
			return nil, sentinel
		},
	})

	var errs, batches int
	for _, err := range call(qs) {
		if err != nil {
			require.ErrorIs(t, err, sentinel)
			errs++
			continue
		}
		batches++
	}
	assert.Equal(t, 1, errs, "exactly one error, then the stream aborts")
	assert.Zero(t, batches, "no batch may be delivered after an OnTraceResult error")
	assert.Equal(t, 1, onResultCalls, "OnTraceResult must not run on batches after it fails")
}

func TestFindTraces_ResultErrorStopsIteration(t *testing.T) {
	assertResultErrorStops(t, func(qs *QueryService) iter.Seq2[[]ptrace.Traces, error] {
		return qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{}))
	})
}

func TestGetTraces_ResultErrorStopsIteration(t *testing.T) {
	assertResultErrorStops(t, func(qs *QueryService) iter.Seq2[[]ptrace.Traces, error] {
		return qs.GetTraces(t.Context(), GetTraceParams{
			TraceIDs:  []tracestore.GetTraceParams{{}},
			RawTraces: true,
		})
	})
}

// TestInterceptedSearch_EarlyStop exercises the "consumer stopped iterating" branches: when the
// range loop breaks, yield returns false and the interception must return rather than pull the
// next batch.
func TestInterceptedSearch_EarlyStop(t *testing.T) {
	t.Run("FindTraces: on a batch", func(t *testing.T) {
		next := &multiBatchReader{
			fakeReader: &fakeReader{},
			batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
		}
		onResultCalls := 0
		qs := interceptedService(next, fakeInterceptor{
			onResultCtx: func(ctx context.Context) context.Context {
				onResultCalls++
				return ctx
			},
		})

		for range qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})) {
			break
		}
		assert.Equal(t, 1, onResultCalls, "the second batch must never be fetched")
	})

	t.Run("FindSpans: on a batch", func(t *testing.T) {
		next := &multiBatchReader{
			fakeReader: &fakeReader{},
			batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
		}
		onResultCalls := 0
		qs := interceptedService(next, fakeInterceptor{
			onResultCtx: func(ctx context.Context) context.Context {
				onResultCalls++
				return ctx
			},
		})

		for range qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})) {
			break
		}
		assert.Equal(t, 1, onResultCalls, "the second batch must never be fetched")
	})

	t.Run("on an error", func(t *testing.T) {
		next := &fakeReader{leadingErr: assert.AnError, batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{})

		for _, err := range qs.GetTraces(t.Context(), GetTraceParams{
			TraceIDs:  []tracestore.GetTraceParams{{}},
			RawTraces: true,
		}) {
			require.ErrorIs(t, err, assert.AnError)
			break
		}
	})

	t.Run("FindSpans: on an error", func(t *testing.T) {
		next := &fakeReader{leadingErr: assert.AnError, batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{})

		for _, err := range qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})) {
			require.ErrorIs(t, err, assert.AnError)
			break
		}
	})
}

func TestFindTraces_ChainAppliesInOrder(t *testing.T) {
	next := &fakeReader{batch: tracesWith("v", "0")}
	var order []string
	first := fakeInterceptor{onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
		order = append(order, "first")
		q.Filter = serviceFilter("first")
		return q, nil
	}}
	second := fakeInterceptor{onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
		order = append(order, "second")
		assert.Equal(t, serviceFilter("first"), q.Filter, "each interceptor sees the previous one's query")
		return q, nil
	}}
	qs := interceptedService(next, first, second)

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, order)
}

type ctxKey struct{}

func TestFindTraces_ThreadsQueryContextToStorageAndResult(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v")}
	var resultSaw any
	qs := interceptedService(next, fakeInterceptor{
		onQueryCtx: func(ctx context.Context) context.Context {
			return context.WithValue(ctx, ctxKey{}, "from-onquery")
		},
		onResultCtx: func(ctx context.Context) context.Context {
			resultSaw = ctx.Value(ctxKey{})
			return ctx
		},
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, "from-onquery", next.gotCtx.Value(ctxKey{}), "the storage reader must see the context OnTraceQuery returned")
	assert.Equal(t, "from-onquery", resultSaw, "OnTraceResult must see the context OnTraceQuery returned")
}

// countingResultCtx records the value each OnTraceResult call is given and increments it, so a test can
// assert the context threads from one batch to the next.
func countingResultCtx(seen *[]int) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		n, _ := ctx.Value(ctxKey{}).(int)
		*seen = append(*seen, n)
		return context.WithValue(ctx, ctxKey{}, n+1)
	}
}

func TestFindTraces_ThreadsResultContextAcrossBatches(t *testing.T) {
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	var seen []int
	qs := interceptedService(next, fakeInterceptor{onResultCtx: countingResultCtx(&seen)})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, seen, "OnTraceResult's returned context must thread into the next batch")
}

func TestFindSpans_AppliesQueryAndResultHooks(t *testing.T) {
	enableStructuredFilters(t)
	narrowedEnd := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	next := &fakeReader{batch: tracesWith("secret", "value"), nextPageToken: "next"}
	next.capabilities = filterCapableBackend()
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
			q.Filter = serviceFilter("gated")
			q.StartTimeMax = narrowedEnd
			return q, nil
		},
		onSpanResult: redactSpanResult("secret"),
	})

	out, err := collectSpans(qs.FindSpans(t.Context(), SpanQueryParams{
		Filter:       serviceFilter("original"),
		StartTimeMin: narrowedEnd.Add(-time.Hour),
		StartTimeMax: narrowedEnd.Add(time.Hour),
	}))
	require.NoError(t, err)
	assert.Equal(t, serviceFilter("gated"), next.gotSpanQuery.Filter, "pre-query hook must reach storage")
	assert.Equal(t, narrowedEnd, next.gotSpanQuery.StartTimeMax, "the narrowed time range must reach storage")
	assert.Equal(t, narrowedEnd.Add(-time.Hour), next.gotSpanQuery.StartTimeMin, "the untouched bound survives the round trip")
	require.Len(t, out, 1)
	assert.Equal(t, "REDACTED", firstSpanAttr(t, []ptrace.Traces{out[0].Results}, "secret"), "result hook must redact")
	assert.Equal(t, tracestore.PageToken("next"), out[0].NextPageToken, "the page token is not the interceptor's to touch")
}

// TestFindSpans_PaginationSurvivesTheInterceptors pins that Pagination is not part of the view an
// interceptor sees and is carried past the hooks unchanged: an interceptor shapes what is
// searched, not how the result is paged.
func TestFindSpans_PaginationSurvivesTheInterceptors(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = filterCapableBackend()
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
			return q, nil
		},
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), SpanQueryParams{
		StartTimeMin: testWindowStart,
		StartTimeMax: testWindowEnd,
		Pagination:   Pagination{PageSize: 10},
	}))
	require.NoError(t, err)
	assert.Equal(t, tracestore.Pagination{PageSize: 10}, next.gotSpanQuery.Pagination, "the page size must reach storage")
}

func TestFindSpans_OnErrorInResponse(t *testing.T) {
	enableStructuredFilters(t)
	onResultCalled := 0
	next := &fakeReader{batch: tracesWith("secret", "value"), err: errors.New("failure")}
	next.capabilities = filterCapableBackend()
	qs := interceptedService(next, fakeInterceptor{
		onSpanResult: func(spans ptrace.Traces) (ptrace.Traces, error) {
			onResultCalled++
			return spans, nil
		},
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), SpanQueryParams{
		Filter: serviceFilter("original"),
	}))
	require.Error(t, err)
	assert.Equal(t, 0, onResultCalled)
}

// TestFindSpans_HandsTheFilterToTheInterceptorAndStorage pins that the caller's filter reaches
// the interceptor verbatim, level qualifiers included, and then reaches a backend that evaluates
// filters unchanged. There is no shape to convert, so nothing is lost on either hop.
func TestFindSpans_HandsTheFilterToTheInterceptorAndStorage(t *testing.T) {
	enableStructuredFilters(t)
	sent := routeFilter()
	var seen queryinterceptor.SpanQuery
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = &tracestore.SearchCapabilities{
		SpanSearch: true,
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan},
			Operators: []expression.Operator{expression.OpEq},
		},
	}
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
			seen = q
			return q, nil
		},
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
		Filter: sent,
	})))
	require.NoError(t, err)
	assert.Equal(t, sent, seen.Filter, "the caller's filter reaches the interceptor verbatim")
	assert.Equal(t, sent, next.gotSpanQuery.Filter, "a reader that evaluates filters keeps one")
	assert.Equal(t, 1, next.capabilityReads, "the capabilities are read once per search")
}

// TestFindSpans_RefusesAFilterTheBackendDoesNotEvaluate pins the case a trace search never has:
// a reader that serves span searches but declares no filter support. A trace search would be
// rewritten into the legacy fields; a span search has no such shape, so the filter is refused
// rather than sent to a reader that never said it could evaluate it.
func TestFindSpans_RefusesAFilterTheBackendDoesNotEvaluate(t *testing.T) {
	enableStructuredFilters(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = &tracestore.SearchCapabilities{SpanSearch: true}
	qs := interceptedService(next)

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
		Filter: serviceFilter("cart"),
	})))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	assert.True(t, IsBadRequest(err))
	assert.False(t, next.findCalled, "storage must not be queried")
}

// TestFindSpans_RefusesAPredicateTheBackendCannotServe pins that the capability check applies to
// the interceptor's output and not only to what the caller sent: the check runs after OnSpanQuery,
// so a predicate an interceptor added is refused on the same terms as the caller's own.
func TestFindSpans_RefusesAPredicateTheBackendCannotServe(t *testing.T) {
	enableStructuredFilters(t)
	spanPredicate := routeFilter()
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = &tracestore.SearchCapabilities{
		WithoutServiceName: true,
		SpanSearch:         true,
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan},
			Operators: []expression.Operator{expression.OpAnd, expression.OpEq},
		},
	}
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: narrowSpansTo(&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
			spanPredicate, serviceFilter("gated"),
		}}),
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
		Filter: spanPredicate,
	})))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	require.ErrorContains(t, err, `does not index the "resource" level`)
	assert.False(t, next.findCalled, "storage must not be queried")
}

// TestFindSpans_RefusesACallerFilterTheDeploymentDoesNotAccept covers the two refusals that
// depend on nothing the backend declared: the feature gate, and a filter that does not finalize.
// Both are the caller's to fix, so both read as a bad request, and neither reaches storage.
func TestFindSpans_RefusesACallerFilterTheDeploymentDoesNotAccept(t *testing.T) {
	t.Run("with the filter gate off", func(t *testing.T) {
		setStructuredFilters(t, false)
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next)

		_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
			Filter: serviceFilter("cart"),
		})))
		require.ErrorIs(t, err, ErrFilterDisabled)
		assert.True(t, IsBadRequest(err))
		assert.False(t, next.findCalled, "storage must not be queried")
	})

	t.Run("with the filter gate off and no filter", func(t *testing.T) {
		setStructuredFilters(t, false)
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next)

		out, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
		require.NoError(t, err, "the gate governs filters, and this search sent none")
		assert.Len(t, out, 1)
	})

	t.Run("a filter that does not finalize", func(t *testing.T) {
		enableStructuredFilters(t)
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next)

		_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
			Filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{serviceFilter("cart")}},
		})))
		require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
		require.ErrorContains(t, err, `operator "and" takes at least two arguments`)
		assert.True(t, IsBadRequest(err))
		assert.False(t, next.findCalled, "storage must not be queried")
	})
}

// TestFindSpans_RefusesAnInvalidInterceptorFilter covers what an interceptor can return that
// storage must not see. Nothing here is the caller's fault, so none of it reads as a bad request,
// and none of it reaches storage — which would answer the malformed trees by matching nothing and
// the missing one by matching everything, silently undoing the narrowing an access-control
// interceptor exists to apply.
func TestFindSpans_RefusesAnInvalidInterceptorFilter(t *testing.T) {
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name:        "a conjunction of one",
			filter:      &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{serviceFilter("gated")}},
			expectedErr: `operator "and" takes at least two arguments`,
		},
		{
			name: "an operator this build does not define",
			filter: &expression.Call{Op: "matches", Args: []expression.Expression{
				&expression.AttributeRef{Key: "a"}, &expression.AnyValue{Value: "b"},
			}},
			expectedErr: `unknown filter operator "matches"`,
		},
		{
			name: "a comparison missing an operand",
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.AnyValue{Value: "a"},
			}},
			expectedErr: `operator "eq" takes`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enableStructuredFilters(t)
			next := &fakeReader{batch: tracesWith("k", "v")}
			qs := interceptedService(next, fakeInterceptor{onSpanQuery: narrowSpansTo(test.filter)})

			_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
				Filter: serviceFilter("original"),
			})))
			require.ErrorIs(t, err, ErrInterceptorFilter)
			require.ErrorContains(t, err, test.expectedErr)
			assert.False(t, next.findCalled, "storage must not be queried")
			assert.False(t, IsBadRequest(err), "the caller's request was fine")
		})
	}
}

// TestFindSpans_RefusesAFilterALaterInterceptorDrops pins that the nil rule holds across the
// chain: a predicate the first interceptor adds is a restriction the second cannot remove, even
// though the query had no filter when the chain started.
func TestFindSpans_RefusesAFilterALaterInterceptorDrops(t *testing.T) {
	enableStructuredFilters(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = filterCapableBackend()
	qs := interceptedService(next,
		fakeInterceptor{onSpanQuery: narrowSpansTo(serviceFilter("gated"))},
		fakeInterceptor{onSpanQuery: narrowSpansTo(nil)},
	)

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.ErrorIs(t, err, ErrInterceptorFilter)
	require.ErrorContains(t, err, "widen the search")
	assert.False(t, next.findCalled, "storage must not be queried")
}

// TestFindSpans_FinalizesAnInterceptorFilter pins that a predicate an interceptor adds reaches
// storage in the same shape as one a caller sent: its constants read against the fields they are
// compared to, and its comparisons turned so the reference comes first. Only checking the structure
// would hand a backend an unread string where it expects a length of time.
func TestFindSpans_FinalizesAnInterceptorFilter(t *testing.T) {
	tests := []struct {
		name     string
		returned *expression.Call
		expected *expression.Call
	}{
		{
			name: "a duration compared against the field that holds one",
			returned: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "2s"},
			}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name: "an instant compared against a timestamp field",
			returned: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "2026-08-18T00:00:00Z"},
			}},
			expected: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.TimestampValue{Value: time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)},
			}},
		},
		{
			name: "a comparison written with the constant on the left",
			returned: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.AnyValue{Value: "2s"},
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
			}},
			expected: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name: "a typed list against an attribute",
			returned: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
			expected: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
		},
		{
			name: "a word one of the closed sets holds",
			returned: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "server"},
			}},
			expected: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
				&expression.StringValue{Value: "server"},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enableStructuredFilters(t)
			next := &fakeReader{batch: tracesWith("k", "v")}
			next.capabilities = filterCapableBackend()
			qs := interceptedService(next, fakeInterceptor{onSpanQuery: narrowSpansTo(test.returned)})

			_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
				Filter: serviceFilter("original"),
			})))
			require.NoError(t, err)
			assert.Equal(t, test.expected, next.gotSpanQuery.Filter)
		})
	}
}

// TestFindSpans_RefusesAnInterceptorConstantThatWillNotParse is the other half: finalizing an
// interceptor's filter can refuse it for the same reason a caller's is refused, and the caller is
// told the interceptor was at fault rather than blamed for its own request.
func TestFindSpans_RefusesAnInterceptorConstantThatWillNotParse(t *testing.T) {
	enableStructuredFilters(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = filterCapableBackend()
	returned := &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
		&expression.AnyValue{Value: "banana"},
	}}
	qs := interceptedService(next, fakeInterceptor{onSpanQuery: narrowSpansTo(returned)})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{
		Filter: serviceFilter("original"),
	})))
	require.ErrorIs(t, err, ErrInterceptorFilter)
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)
	assert.False(t, next.findCalled, "storage must not be queried")
	assert.False(t, IsBadRequest(err), "the caller's request was fine")
}

// TestFindSpans_AllowsNoFilterForAPredicatelessQuery is the other side of the nil rule: a search
// of the time range alone has no filter to begin with, so an interceptor that leaves it that way
// has widened nothing and the search proceeds.
func TestFindSpans_AllowsNoFilterForAPredicatelessQuery(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v")}
	var seen queryinterceptor.SpanQuery
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
			seen = q
			return q, nil
		},
	})

	out, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Nil(t, seen.Filter, "there were no predicates to show")
	assert.True(t, next.findCalled)
}

// TestFindSpans_RefusedByATraceOnlyInterceptor pins the fail-closed mixin end to end: an
// interceptor that embeds queryinterceptor.UnsupportedSpanSearch makes the query service refuse
// a span search before storage sees it, and the refusal is a deployment fault, not a bad request.
// The interceptor package's ErrSpanSearchUnsupported is a different sentinel from this package's,
// which names a backend that cannot serve the search and is a bad request.
func TestFindSpans_RefusedByATraceOnlyInterceptor(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, traceOnlyInterceptor{})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.ErrorIs(t, err, queryinterceptor.ErrSpanSearchUnsupported)
	require.NotErrorIs(t, err, ErrSpanSearchUnsupported, "the backend was not the one refusing")
	assert.False(t, IsBadRequest(err), "the caller's request was fine")
	assert.False(t, next.findCalled, "storage must not be queried")
}

// traceOnlyInterceptor is the shape of an implementation written before span search existed: it
// gates trace searches and embeds the mixin for the rest.
type traceOnlyInterceptor struct {
	queryinterceptor.UnsupportedSpanSearch
}

func (traceOnlyInterceptor) OnTraceQuery(ctx context.Context, q queryinterceptor.TraceQuery) (context.Context, queryinterceptor.TraceQuery, error) {
	return ctx, q, nil
}

func (traceOnlyInterceptor) OnTraceResult(ctx context.Context, t []ptrace.Traces) (context.Context, []ptrace.Traces, error) {
	return ctx, t, nil
}

func TestFindSpans_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) { return q, sentinel },
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.ErrorIs(t, err, sentinel)
	assert.False(t, next.findCalled, "storage must not be queried when the query is rejected")
}

func TestFindSpans_ResultErrorAborts(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onSpanResult: func(ptrace.Traces) (ptrace.Traces, error) { return ptrace.Traces{}, sentinel },
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.ErrorIs(t, err, sentinel)
}

// TestFindSpans_PassesAReaderErrorThrough pins that an error the reader yields is handed to the
// caller as it arrived, and that a caller who reads past it still gets the chunks that follow.
func TestFindSpans_PassesAReaderErrorThrough(t *testing.T) {
	next := &fakeReader{leadingErr: assert.AnError, batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{})

	var errs, chunks int
	for _, err := range qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})) {
		if err != nil {
			require.ErrorIs(t, err, assert.AnError)
			errs++
			continue
		}
		chunks++
	}
	assert.Equal(t, 1, errs)
	assert.Equal(t, 1, chunks, "the chunk after the error is still delivered")
}

func TestFindSpans_ResultErrorStopsIteration(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	onResultCalls := 0
	qs := interceptedService(next, fakeInterceptor{
		onSpanResult: func(ptrace.Traces) (ptrace.Traces, error) {
			onResultCalls++
			return ptrace.Traces{}, sentinel
		},
	})

	var errs, chunks int
	for _, err := range qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})) {
		if err != nil {
			require.ErrorIs(t, err, sentinel)
			errs++
			continue
		}
		chunks++
	}
	assert.Equal(t, 1, errs, "exactly one error, then the stream aborts")
	assert.Zero(t, chunks, "no chunk may be delivered after an OnSpanResult error")
	assert.Equal(t, 1, onResultCalls, "OnSpanResult must not run on chunks after it fails")
}

func TestFindSpans_ChainAppliesInOrder(t *testing.T) {
	next := &fakeReader{batch: tracesWith("v", "0")}
	next.capabilities = filterCapableBackend()
	var order []string
	first := fakeInterceptor{onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
		order = append(order, "first")
		q.Filter = serviceFilter("first")
		return q, nil
	}}
	second := fakeInterceptor{onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
		order = append(order, "second")
		assert.Equal(t, serviceFilter("first"), q.Filter, "each interceptor sees the previous one's query")
		return q, nil
	}}
	qs := interceptedService(next, first, second)

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, order)
}

func TestFindSpans_ThreadsQueryContextToStorageAndResult(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v")}
	var resultSaw any
	qs := interceptedService(next, fakeInterceptor{
		onQueryCtx: func(ctx context.Context) context.Context {
			return context.WithValue(ctx, ctxKey{}, "from-onquery")
		},
		onResultCtx: func(ctx context.Context) context.Context {
			resultSaw = ctx.Value(ctxKey{})
			return ctx
		},
	})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, "from-onquery", next.gotCtx.Value(ctxKey{}), "the storage reader must see the context OnSpanQuery returned")
	assert.Equal(t, "from-onquery", resultSaw, "OnSpanResult must see the context OnSpanQuery returned")
}

func TestFindSpans_ThreadsResultContextAcrossBatches(t *testing.T) {
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	var seen []int
	qs := interceptedService(next, fakeInterceptor{onResultCtx: countingResultCtx(&seen)})

	_, err := collectSpans(qs.FindSpans(t.Context(), searchSpansQuery(SpanQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, seen, "OnSpanResult's returned context must thread into the next batch")
}

func collectSpans(it iter.Seq2[tracestore.PageChunk[ptrace.Traces], error]) ([]tracestore.PageChunk[ptrace.Traces], error) {
	var out []tracestore.PageChunk[ptrace.Traces]
	for batch, err := range it {
		if err != nil {
			return out, err
		}
		out = append(out, batch)
	}
	return out, nil
}

func TestGetTraces_ThreadsResultContextAcrossBatches(t *testing.T) {
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	var seen []int
	qs := interceptedService(next, fakeInterceptor{onResultCtx: countingResultCtx(&seen)})

	_, err := collectTraces(qs.GetTraces(t.Context(), GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{}},
		RawTraces: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, seen, "OnTraceResult's returned context must thread into the next batch")
}

func TestFindTraceSummaries_AppliesQueryHook(t *testing.T) {
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	qs := interceptedService(next, fakeInterceptor{onQuery: narrowTo(serviceFilter("gated"))})

	var got [][]tracestore.TraceSummary
	for chunk, err := range qs.FindTraceSummaries(t.Context(), searchQuery(TraceQueryParams{
		ServiceName: "original",
	})) {
		require.NoError(t, err)
		got = append(got, chunk.Results)
	}
	assert.Equal(t, "gated", next.gotSummaryQuery.ServiceName, "pre-query hook must reach storage")
	require.Len(t, got, 1)
	require.Len(t, got[0], 1)
	assert.Equal(t, "svc", got[0][0].RootServiceName)
}

func TestFindTraceSummaries_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{summaries: []tracestore.TraceSummary{{}}}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) { return q, sentinel },
	})

	var err error
	for _, e := range qs.FindTraceSummaries(t.Context(), searchQuery(TraceQueryParams{})) {
		err = e
	}
	require.ErrorIs(t, err, sentinel)
	assert.False(t, next.summaryCalled, "storage must not be queried when the query is rejected")
}

// TestFindTraceSummaries_FallbackAppliesResultHook covers the backends that cannot summarize
// natively: the fallback loads whole traces, so an interceptor gets its say over them before they
// are summarized, and it gets that say once rather than on both the summary search and the
// fallback.
func TestFindTraceSummaries_FallbackAppliesResultHook(t *testing.T) {
	next := &fakeReader{
		summaryErr: fmt.Errorf("no native summaries: %w", errors.ErrUnsupported),
		batch:      tracesWith("k", "v"),
	}
	onQueryCalls := 0
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
			onQueryCalls++
			return q, nil
		},
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, assert.AnError },
	})

	var err error
	for _, e := range qs.FindTraceSummaries(t.Context(), searchQuery(TraceQueryParams{})) {
		err = e
	}
	require.ErrorIs(t, err, assert.AnError, "the fallback's traces pass through OnTraceResult")
	assert.Equal(t, 1, onQueryCalls, "the fallback reuses the query the interceptor already saw")
}

// TestInterceptorRunsWithTheFilterGateOff pins the interaction between the two opt-ins: an
// interceptor is configured, jaeger.query.structuredFilters is off, and a caller sends the legacy
// predicate fields. The interceptor still sees every predicate as a filter and can narrow it,
// because the gate guards a filter a *caller* sent and this query carries none. Anyone running an
// interceptor therefore needs no second opt-in, and the deployment still accepts no filter over
// api_v3.
func TestInterceptorRunsWithTheFilterGateOff(t *testing.T) {
	setStructuredFilters(t, false)

	var seen queryinterceptor.TraceQuery
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
			seen = q
			q.Filter = serviceFilter("gated")
			return q, nil
		},
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(TraceQueryParams{
		ServiceName:  "original",
		Attributes:   attributesWith("http.method", "GET"),
		StartTimeMin: time.Now().Add(-time.Hour),
		StartTimeMax: time.Now(),
	})))
	require.NoError(t, err)

	require.NotNil(t, seen.Filter, "the interceptor is shown predicates, gate or no gate")
	assert.Equal(t, expression.OpAnd, seen.Filter.Op)
	assert.Len(t, seen.Filter.Args, 2, "the service and the tag both became predicates")
	assert.Equal(t, "gated", next.gotQuery.ServiceName, "storage gets what the interceptor chose")
	assert.Nil(t, next.gotQuery.Filter, "in the shape this backend supports")
}
