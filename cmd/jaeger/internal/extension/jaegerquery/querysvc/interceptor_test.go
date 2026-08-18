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
}

// SearchCapabilities answers for a backend that searches every service and evaluates no filter
// unless a test says otherwise, since that is the shape most of these searches assume.
func (f *fakeReader) SearchCapabilities(context.Context) (tracestore.SearchCapabilities, error) {
	if f.capabilities == nil {
		return tracestore.SearchCapabilities{WithoutServiceName: true}, nil
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

func (*fakeReader) FindTraceIDs(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(func([]tracestore.FoundTraceID, error) bool) {}
}

func (*fakeReader) GetServices(context.Context) ([]string, error) {
	return []string{"svc"}, nil
}

func (*fakeReader) GetOperations(context.Context, tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return []tracestore.Operation{{Name: "op"}}, nil
}

func (f *fakeReader) FindTraceSummaries(_ context.Context, q tracestore.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
	f.summaryCalled = true
	f.gotSummaryQuery = q
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		if f.summaryErr != nil {
			yield(nil, f.summaryErr)
			return
		}
		yield(f.summaries, nil)
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

// fakeInterceptor lets each test supply the hook behavior it needs. It receives the public Query,
// exactly as a real interceptor would. The optional onQueryCtx and onResultCtx hooks transform
// (and observe) the context, so a test can assert how the query service threads it from OnQuery
// into the reader and OnResult.
type fakeInterceptor struct {
	onQuery     func(queryinterceptor.Query) (queryinterceptor.Query, error)
	onResult    func([]ptrace.Traces) ([]ptrace.Traces, error)
	onQueryCtx  func(context.Context) context.Context
	onResultCtx func(context.Context) context.Context
}

func (f fakeInterceptor) OnQuery(ctx context.Context, q queryinterceptor.Query) (context.Context, queryinterceptor.Query, error) {
	if f.onQueryCtx != nil {
		ctx = f.onQueryCtx(ctx)
	}
	if f.onQuery != nil {
		nq, err := f.onQuery(q)
		return ctx, nq, err
	}
	return ctx, q, nil
}

func (f fakeInterceptor) OnResult(ctx context.Context, t []ptrace.Traces) (context.Context, []ptrace.Traces, error) {
	if f.onResultCtx != nil {
		ctx = f.onResultCtx(ctx)
	}
	if f.onResult != nil {
		nt, err := f.onResult(t)
		return ctx, nt, err
	}
	return ctx, t, nil
}

// interceptedService builds a query service that runs the given interceptors over next.
func interceptedService(next tracestore.Reader, interceptors ...queryinterceptor.Interceptor) *QueryService {
	return NewQueryService(next, nil, QueryServiceOptions{Interceptors: interceptors})
}

// searchQuery asks for raw traces, so that the batches a test asserts on are the ones the reader
// yielded and the interceptor rewrote, rather than the aggregated traces built from them.
func searchQuery(q tracestore.TraceQueryParams) TraceQueryParams {
	return TraceQueryParams{TraceQueryParams: q, RawTraces: true}
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

func narrowTo(filter *expression.Call) func(queryinterceptor.Query) (queryinterceptor.Query, error) {
	return func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
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

	out, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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
		var seen queryinterceptor.Query
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
				seen = q
				q.Filter = serviceFilter("gated")
				return q, nil
			},
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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
		var seen queryinterceptor.Query
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
				seen = q
				return q, nil
			},
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
			Filter: filter,
		})))
		require.NoError(t, err)
		assert.Equal(t, filter, next.gotQuery.Filter, "a reader that evaluates filters keeps one")
		assert.Empty(t, next.gotQuery.ServiceName)
	})
}

// TestFindTraces_RefusesAPredicateTheBackendCannotServe pins that the capability check applies to
// the interceptor's output and not only to what the caller sent: the query service converts once,
// after OnQuery, so a predicate an interceptor added is refused on the same terms as the caller's
// own.
func TestFindTraces_RefusesAPredicateTheBackendCannotServe(t *testing.T) {
	t.Run("the backend evaluates no filter and the fields cannot carry it", func(t *testing.T) {
		next := &fakeReader{batch: tracesWith("k", "v")}
		qs := interceptedService(next, fakeInterceptor{
			onQuery: narrowTo(&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
				serviceFilter("a"), serviceFilter("b"),
			}}),
		})

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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

		_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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
			expectedErr: "widen the search to every trace in the time range",
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

			_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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
		Filter: &tracestore.FilterCapabilities{
			Levels: []expression.Level{expression.LevelSpan},
			Operators: []expression.Operator{
				expression.OpEq, expression.OpGt, expression.OpLt, expression.OpIn,
			},
		},
	}
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

			_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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
	var seen queryinterceptor.Query
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
			seen = q
			return q, nil
		},
	})

	out, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})))
	require.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Nil(t, seen.Filter, "there were no predicates to show")
	assert.True(t, next.findCalled)
}

func TestFindTraces_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) { return q, sentinel },
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})))
	require.ErrorIs(t, err, sentinel)
	assert.False(t, next.findCalled, "storage must not be queried when the query is rejected")
}

func TestFindTraces_ResultErrorAborts(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, sentinel },
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})))
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

// assertResultErrorStops verifies that an OnResult failure aborts the stream: even a consumer
// that keeps ranging after the error must never receive a later batch, and OnResult must not run
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
	assert.Zero(t, batches, "no batch may be delivered after an OnResult error")
	assert.Equal(t, 1, onResultCalls, "OnResult must not run on batches after it fails")
}

func TestFindTraces_ResultErrorStopsIteration(t *testing.T) {
	assertResultErrorStops(t, func(qs *QueryService) iter.Seq2[[]ptrace.Traces, error] {
		return qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{}))
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
	t.Run("on a batch", func(t *testing.T) {
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

		for range qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})) {
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
}

func TestFindTraces_ChainAppliesInOrder(t *testing.T) {
	next := &fakeReader{batch: tracesWith("v", "0")}
	var order []string
	first := fakeInterceptor{onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
		order = append(order, "first")
		q.Filter = serviceFilter("first")
		return q, nil
	}}
	second := fakeInterceptor{onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
		order = append(order, "second")
		assert.Equal(t, serviceFilter("first"), q.Filter, "each interceptor sees the previous one's query")
		return q, nil
	}}
	qs := interceptedService(next, first, second)

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})))
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

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, "from-onquery", next.gotCtx.Value(ctxKey{}), "the storage reader must see the context OnQuery returned")
	assert.Equal(t, "from-onquery", resultSaw, "OnResult must see the context OnQuery returned")
}

// countingResultCtx records the value each OnResult call is given and increments it, so a test can
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

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{})))
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, seen, "OnResult's returned context must thread into the next batch")
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
	assert.Equal(t, []int{0, 1}, seen, "OnResult's returned context must thread into the next batch")
}

func TestFindTraceSummaries_AppliesQueryHook(t *testing.T) {
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	qs := interceptedService(next, fakeInterceptor{onQuery: narrowTo(serviceFilter("gated"))})

	var got [][]tracestore.TraceSummary
	for s, err := range qs.FindTraceSummaries(t.Context(), searchQuery(tracestore.TraceQueryParams{
		ServiceName: "original",
	})) {
		require.NoError(t, err)
		got = append(got, s)
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
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) { return q, sentinel },
	})

	var err error
	for _, e := range qs.FindTraceSummaries(t.Context(), searchQuery(tracestore.TraceQueryParams{})) {
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
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
			onQueryCalls++
			return q, nil
		},
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, assert.AnError },
	})

	var err error
	for _, e := range qs.FindTraceSummaries(t.Context(), searchQuery(tracestore.TraceQueryParams{})) {
		err = e
	}
	require.ErrorIs(t, err, assert.AnError, "the fallback's traces pass through OnResult")
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

	var seen queryinterceptor.Query
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
			seen = q
			q.Filter = serviceFilter("gated")
			return q, nil
		},
	})

	_, err := collectTraces(qs.FindTraces(t.Context(), searchQuery(tracestore.TraceQueryParams{
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
