// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// The structured query filter of RFC 0005 is checked against a live backend here, because what a
// unit test can check is the query a filter lowers to, not the traces that query comes back with.
// The two differ in every way a schema can surprise the reader: which level an attribute was
// written at, whether a duration survived as a number, what the write path did with an event's
// name.
//
// filterCorpus is the whole trace corpus every case searches, so each case has counterexamples the
// backend must leave out rather than one trace it can hardly miss. It is small and deliberate: the
// attribute key `zone` sits at the span level in one trace, the resource level in another and the
// event level in a third, so a reader that ignores the level a predicate names fails here instead
// of passing with a superset.
var filterCorpus = []string{
	// filter-cart, GET /cart, 1ms, span zone=us-east, span http.status_code=200
	"filter_cart_get_trace",
	// filter-cart, POST /cart/items, 2s, span http.status_code=500, event "exception"
	"filter_cart_post_trace",
	// filter-checkout, GET /checkout, 40ms, resource zone=us-east, span http.status_code=200
	"filter_checkout_trace",
	// filter-search, GET /search, 5ms, no span attributes, event "cache.miss" with zone=eu-west
	"filter_search_trace",
}

// filterSearchDepth is larger than the corpus, so a case's result set is what the filter matched
// rather than what the search depth cut it down to.
const filterSearchDepth = 100

// filterCase is one search expressed as a filter, and the traces of the corpus it must return.
type filterCase struct {
	caption  string
	filter   *expression.Call
	expected []string
}

// filterTestCases covers what M4 of RFC 0005 delivered for Elasticsearch and OpenSearch: the three
// levels those index, the built-in fields they route to a field of their own, every operator they
// declare, and boolean composition over the lot.
func filterTestCases(p builder.Predicate) []filterCase {
	return []filterCase{
		{
			caption:  "a span-level attribute matches the span's own attributes only",
			filter:   p.Span().Attr("zone").Eq("us-east"),
			expected: []string{"filter_cart_get_trace"},
		},
		{
			caption:  "a resource-level attribute matches the resource's attributes only",
			filter:   p.Resource().Attr("zone").Eq("us-east"),
			expected: []string{"filter_checkout_trace"},
		},
		{
			caption:  "an unqualified attribute matches either the span or the resource",
			filter:   p.Attr("zone").Eq("us-east"),
			expected: []string{"filter_cart_get_trace", "filter_checkout_trace"},
		},
		{
			caption:  "an event-level attribute matches an attribute of one of the span's events",
			filter:   p.Event().Attr("zone").Eq("eu-west"),
			expected: []string{"filter_search_trace"},
		},
		{
			// The span-or-resource default of RFC 0005 §5.1, asserted by what it leaves out: the
			// search trace carries `zone` on an event and nowhere else.
			caption:  "an unqualified attribute does not reach the event level",
			filter:   p.Attr("zone").Exists(),
			expected: []string{"filter_cart_get_trace", "filter_checkout_trace"},
		},
		{
			caption:  "the service name",
			filter:   p.Resource().Service.Eq("filter-checkout"),
			expected: []string{"filter_checkout_trace"},
		},
		{
			caption:  "the service name against a list of names",
			filter:   p.Resource().Service.In("filter-checkout", "filter-search"),
			expected: []string{"filter_checkout_trace", "filter_search_trace"},
		},
		{
			caption:  "the operation name",
			filter:   p.Span().Name.Eq("GET /cart"),
			expected: []string{"filter_cart_get_trace"},
		},
		{
			caption:  "a pattern on the operation name matches anywhere in it",
			filter:   p.Span().Name.Matches("cart"),
			expected: []string{"filter_cart_get_trace", "filter_cart_post_trace"},
		},
		{
			// The write path stores an event's name as an attribute of the event, so this asserts
			// that a filter naming the field finds what the write path recorded.
			caption:  "the name of one of the span's events",
			filter:   p.Event().Name.Eq("exception"),
			expected: []string{"filter_cart_post_trace"},
		},
		{
			caption:  "a duration greater than a bound",
			filter:   p.Span().Duration.Gt(time.Second),
			expected: []string{"filter_cart_post_trace"},
		},
		{
			caption:  "a duration at most a bound",
			filter:   p.Span().Duration.Lte(5 * time.Millisecond),
			expected: []string{"filter_cart_get_trace", "filter_search_trace"},
		},
		{
			caption: "a duration between two bounds",
			filter: p.And(
				p.Span().Duration.Gte(5*time.Millisecond),
				p.Span().Duration.Lte(40*time.Millisecond),
			),
			expected: []string{"filter_checkout_trace", "filter_search_trace"},
		},
		{
			// An inequality asks for the spans that hold the attribute and hold something else
			// (RFC 0005 §5.3), so the search trace, which carries no http.status_code at all, is
			// not among them.
			caption:  "an attribute inequality leaves out a span that lacks the attribute",
			filter:   p.Span().Attr("http.status_code").Ne("200"),
			expected: []string{"filter_cart_post_trace"},
		},
		{
			caption:  "an attribute exists",
			filter:   p.Span().Attr("http.status_code").Exists(),
			expected: []string{"filter_cart_get_trace", "filter_cart_post_trace", "filter_checkout_trace"},
		},
		{
			caption:  "a pattern on an attribute value",
			filter:   p.Span().Attr("http.status_code").Matches("5.."),
			expected: []string{"filter_cart_post_trace"},
		},
		{
			caption: "a conjunction of a built-in field and a duration",
			filter: p.And(
				p.Resource().Service.Eq("filter-cart"),
				p.Span().Duration.Gt(time.Second),
			),
			expected: []string{"filter_cart_post_trace"},
		},
		{
			caption: "a disjunction",
			filter: p.Or(
				p.Resource().Service.Eq("filter-search"),
				p.Span().Name.Eq("GET /cart"),
			),
			expected: []string{"filter_cart_get_trace", "filter_search_trace"},
		},
		{
			caption: "a negation narrowing a conjunction",
			filter: p.And(
				p.Resource().Attr("deployment.environment").Eq("staging"),
				p.Not(p.Resource().Service.Eq("filter-search")),
			),
			expected: []string{"filter_checkout_trace"},
		},
		{
			caption: "a disjunction nested inside a conjunction",
			filter: p.And(
				p.Resource().Attr("deployment.environment").Eq("prod"),
				p.Or(
					p.Span().Attr("http.status_code").Eq("500"),
					p.Span().Attr("zone").Eq("us-east"),
				),
			),
			expected: []string{"filter_cart_get_trace", "filter_cart_post_trace"},
		},
	}
}

// testFindTracesWithFilter searches the corpus with a filter instead of the legacy predicate
// fields. A filter stands alone — a query carrying one beside a service name or a tag map is
// refused — so the service, the operation name and the duration bounds are predicates of the
// filter here, and only the time range and the search depth are left outside it.
func (s *StorageIntegration) testFindTracesWithFilter(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	s.requireReaderEvaluatesFilters(t)
	corpus := s.writeFilterCorpus(t)
	start, end := filterCorpusTimeRange(corpus)
	var p builder.Predicate

	// A deployment that will not serve a filter at all refuses every case below, and a search that
	// comes back with an error is retried for a minute and a half before it is called a failure —
	// which for a whole battery is half an hour of a CI run spent on one misconfiguration. So the
	// first filter goes straight to the reader, whose error says what is actually wrong.
	s.requireFilterIsServed(t, filterQuery(p.Resource().Service.Exists(), start, end))

	for _, testCase := range filterTestCases(p) {
		t.Run(testCase.caption, func(t *testing.T) {
			s.skipIfNeeded(t)
			expected := make([]ptrace.Traces, 0, len(testCase.expected))
			for _, name := range testCase.expected {
				expected = append(expected, corpus[name])
			}
			actual := s.findTracesByQuery(t, filterQuery(testCase.filter, start, end), expected)
			CompareTraceSlices(t, expected, actual)
		})
	}

	// RFC 0005 §7 promises that a backend either evaluates a predicate or refuses it, and never
	// answers a predicate it cannot evaluate with a wider result set. These two are the shapes an
	// Elasticsearch or OpenSearch reader declares it cannot serve: the scope level, which its
	// schema does not index apart from the span's own attributes, and the `some` quantifier.
	refusals := []struct {
		caption string
		filter  *expression.Call
		names   string
	}{
		{
			caption: "a level the backend does not index is refused",
			filter:  p.Scope().Attr("library.tier").Eq("core"),
			names:   "scope",
		},
		{
			caption: "an operator the backend does not evaluate is refused",
			filter:  p.Some(p.Event(), p.Event().Name.Eq("exception")),
			names:   "some",
		},
	}
	for _, refusal := range refusals {
		t.Run(refusal.caption, func(t *testing.T) {
			s.skipIfNeeded(t)
			query := filterQuery(refusal.filter, start, end)
			_, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
				s.TraceReader.FindTraces(context.Background(), *query),
			))
			require.Error(t, err, "the search must be refused rather than answered with a wider result set")
			// The sentinel error does not survive the gRPC hop the e2e tests read through, so the
			// assertion is on what the message names, which does.
			require.ErrorContains(t, err, refusal.names)
		})
	}
}

// RunFilterRewriteTest asks one question both ways — through the legacy predicate fields, and as
// the filter that means the same thing — and requires the two to answer alike. What it checks is
// the query service's rewrite: a backend that declares no filter capability is handed the filter
// expressed in the legacy fields instead (ToLegacyShape), and a rewrite that dropped a predicate
// would answer with more traces than were asked for.
//
// It is not part of RunSpanStoreTests, because the rewrite is the query service's and a suite that
// wires up a Reader itself has none: such a Reader is never handed a filter in production, and
// handed one here it ignores the field and answers with every trace in the time range. So one e2e
// suite over a backend that evaluates no filter calls this, and that is enough — the rewrite reads
// the same for every backend, and both halves of the comparison go through the same reader, so a
// second backend would only re-run the same rewrite.
//
// The two result sets are compared against each other rather than against the fixtures, so a
// backend whose read path reshapes a trace is still held to answering both forms the same way.
func (s *StorageIntegration) RunFilterRewriteTest(t *testing.T) {
	t.Run("FilterRewrittenAsLegacyQuery", func(t *testing.T) {
		defer s.cleanUp(t)

		corpus := s.writeFilterCorpus(t)
		start, end := filterCorpusTimeRange(corpus)
		var p builder.Predicate

		legacyAttributes := pcommon.NewMap()
		legacyAttributes.PutStr("http.status_code", "500")
		legacy := &tracestore.TraceQueryParams{
			ServiceName:  "filter-cart",
			Attributes:   legacyAttributes,
			StartTimeMin: start,
			StartTimeMax: end,
			SearchDepth:  filterSearchDepth,
		}
		filter := filterQuery(p.And(
			p.Resource().Service.Eq("filter-cart"),
			p.Attr("http.status_code").Eq("500"),
		), start, end)

		expected := []ptrace.Traces{corpus["filter_cart_post_trace"]}
		viaLegacy := s.findTracesByQuery(t, legacy, expected)
		viaFilter := s.findTracesByQuery(t, filter, expected)
		require.NotEmpty(t, viaLegacy, "the legacy query must match a trace, or the two agreeing says nothing")
		CompareTraceSlices(t, viaLegacy, viaFilter)
	})
}

// filterQuery is a search whose only predicate is the filter. The attributes map is empty rather
// than unset because a Reader is entitled to read it (it "must be initialized with
// pcommon.NewMap() before use"), and a filter must arrive alone: a query carrying one beside a
// service name, an operation name, a tag or a duration bound is refused.
func filterQuery(filter *expression.Call, start, end time.Time) *tracestore.TraceQueryParams {
	return &tracestore.TraceQueryParams{
		Filter:       filter,
		Attributes:   pcommon.NewMap(),
		StartTimeMin: start,
		StartTimeMax: end,
		SearchDepth:  filterSearchDepth,
	}
}

// requireReaderEvaluatesFilters fails the battery at once where the reader has not declared it can
// evaluate a filter, rather than leaving each case to retry a wrong answer for a minute and a half.
// Such a backend belongs in the capabilities skip list, and the failure says so.
//
// A reader that cannot report its capabilities is taken at its word and left to the cases: that is
// what the api_v3 client the e2e suites read through answers, having no way to ask the query
// service what is behind it.
func (s *StorageIntegration) requireReaderEvaluatesFilters(t *testing.T) {
	caps, err := s.TraceReader.SearchCapabilities(context.Background())
	if err != nil {
		return
	}
	require.False(t, caps.Filter.IsEmpty(),
		"this reader declares no filter capabilities, so it cannot answer the filter battery; "+
			"excuse the backend from it in internal/storage/integration/capabilities")
}

// requireFilterIsServed fails the whole battery, rather than each case in it, when the deployment
// will not serve a filter at all.
func (s *StorageIntegration) requireFilterIsServed(t *testing.T, query *tracestore.TraceQueryParams) {
	_, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
		s.TraceReader.FindTraces(context.Background(), *query),
	))
	require.NoError(t, err, "this deployment refuses the filter itself, so no case below can pass")
}

// writeFilterCorpus writes every trace of the corpus and returns them by fixture name.
func (s *StorageIntegration) writeFilterCorpus(t *testing.T) map[string]ptrace.Traces {
	corpus := make(map[string]ptrace.Traces, len(filterCorpus))
	for _, name := range filterCorpus {
		trace := s.getTraceFixture(t, name)
		s.writeTrace(t, trace)
		corpus[name] = trace
	}
	return corpus
}

// filterCorpusTimeRange is a range around the whole corpus, so that every case is answered by its
// filter rather than by the time range narrowing the corpus first. The bounds are derived from the
// traces as written, because loading a fixture moves its timestamps to a recent date.
func filterCorpusTimeRange(corpus map[string]ptrace.Traces) (earliest, latest time.Time) {
	for _, trace := range corpus {
		for _, span := range jptrace.SpanIter(trace) {
			start := span.StartTimestamp().AsTime()
			if earliest.IsZero() || start.Before(earliest) {
				earliest = start
			}
			if latest.IsZero() || start.After(latest) {
				latest = start
			}
		}
	}
	return earliest.Add(-time.Minute), latest.Add(time.Minute)
}
