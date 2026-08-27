// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"strings"
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
// filterCorpusDir holds every trace the cases search, and all of it is loaded, so a fixture added
// there is searched without being named anywhere else and none can be left orphaned. The corpus is
// small and deliberate: the attribute key `zone` sits at the span level in one trace, the resource
// level in another and the event level in a third, so a reader that ignores the level a predicate
// names fails here instead of passing with a superset.
const filterCorpusDir = "fixtures/traces/filter"

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
			expected: []string{"cart_get"},
		},
		{
			caption:  "a resource-level attribute matches the resource's attributes only",
			filter:   p.Resource().Attr("zone").Eq("us-east"),
			expected: []string{"checkout"},
		},
		{
			caption:  "an unqualified attribute matches either the span or the resource",
			filter:   p.Attr("zone").Eq("us-east"),
			expected: []string{"cart_get", "checkout"},
		},
		{
			caption:  "an event-level attribute matches an attribute of one of the span's events",
			filter:   p.Event().Attr("zone").Eq("eu-west"),
			expected: []string{"search"},
		},
		{
			// The span-or-resource default of RFC 0005 §5.1, asserted by what it leaves out: the
			// search trace carries `zone` on an event and nowhere else.
			caption:  "an unqualified attribute does not reach the event level",
			filter:   p.Attr("zone").Exists(),
			expected: []string{"cart_get", "checkout"},
		},
		{
			caption:  "the service name",
			filter:   p.Resource().Service.Eq("filter-checkout"),
			expected: []string{"checkout"},
		},
		{
			caption:  "the service name against a list of names",
			filter:   p.Resource().Service.In("filter-checkout", "filter-search"),
			expected: []string{"checkout", "search"},
		},
		{
			caption:  "the operation name",
			filter:   p.Span().Name.Eq("GET /cart"),
			expected: []string{"cart_get"},
		},
		{
			caption:  "a pattern on the operation name matches anywhere in it",
			filter:   p.Span().Name.Matches("cart"),
			expected: []string{"cart_get", "cart_post"},
		},
		{
			// The write path stores an event's name as an attribute of the event, so this asserts
			// that a filter naming the field finds what the write path recorded.
			caption:  "the name of one of the span's events",
			filter:   p.Event().Name.Eq("exception"),
			expected: []string{"cart_post"},
		},
		{
			caption:  "a duration greater than a bound",
			filter:   p.Span().Duration.Gt(time.Second),
			expected: []string{"cart_post"},
		},
		{
			caption:  "a duration at most a bound",
			filter:   p.Span().Duration.Lte(5 * time.Millisecond),
			expected: []string{"cart_get", "search"},
		},
		{
			caption: "a duration between two bounds",
			filter: p.And(
				p.Span().Duration.Gte(5*time.Millisecond),
				p.Span().Duration.Lte(40*time.Millisecond),
			),
			expected: []string{"checkout", "search"},
		},
		{
			// An inequality asks for the spans that hold the attribute and hold something else
			// (RFC 0005 §5.3), so the search trace, which carries no http.status_code at all, is
			// not among them.
			caption:  "an attribute inequality leaves out a span that lacks the attribute",
			filter:   p.Span().Attr("http.status_code").Ne("200"),
			expected: []string{"cart_post"},
		},
		{
			// Ordering an attribute needs the value stored as a number. A backend that indexes
			// attributes as text refuses this instead, and excuses itself from this case — see
			// its twin among the refusals below.
			caption:  "ordering compares a numeric attribute as a number",
			filter:   p.Span().Attr("retry.count").Gt(3),
			expected: []string{"cart_post"},
		},
		{
			caption:  "an attribute exists",
			filter:   p.Span().Attr("http.status_code").Exists(),
			expected: []string{"cart_get", "cart_post", "checkout"},
		},
		{
			caption:  "a pattern on an attribute value",
			filter:   p.Span().Attr("http.status_code").Matches("5.."),
			expected: []string{"cart_post"},
		},
		{
			caption: "a conjunction of a built-in field and a duration",
			filter: p.And(
				p.Resource().Service.Eq("filter-cart"),
				p.Span().Duration.Gt(time.Second),
			),
			expected: []string{"cart_post"},
		},
		{
			caption: "a disjunction",
			filter: p.Or(
				p.Resource().Service.Eq("filter-search"),
				p.Span().Name.Eq("GET /cart"),
			),
			expected: []string{"cart_get", "search"},
		},
		{
			caption: "a negation narrowing a conjunction",
			filter: p.And(
				p.Resource().Attr("deployment.environment").Eq("staging"),
				p.Not(p.Resource().Service.Eq("filter-search")),
			),
			expected: []string{"checkout"},
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
			expected: []string{"cart_get", "cart_post"},
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
			expected := filterCorpusTraces(t, corpus, testCase.expected)
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
		{
			// The twin of "ordering compares a numeric attribute as a number" above: the same
			// filter, for a backend whose schema indexes an attribute as text. This one refuses
			// inside the reader rather than at the capability edge, because the operator and the
			// level are both declared and only the pairing is unservable. A backend that orders
			// attributes natively excuses itself from this case and runs the other.
			caption: "ordering an attribute is refused where it is indexed as text",
			filter:  p.Span().Attr("retry.count").Gt(3),
			names:   "keyword rather than a number",
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

		expected := filterCorpusTraces(t, corpus, []string{"cart_post"})
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

// requireFilterIsServed fails the whole battery, rather than each case in it, when the deployment
// will not serve a filter at all.
func (s *StorageIntegration) requireFilterIsServed(t *testing.T, query *tracestore.TraceQueryParams) {
	_, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
		s.TraceReader.FindTraces(context.Background(), *query),
	))
	require.NoError(t, err, "this deployment refuses the filter itself, so no case below can pass")
}

// writeFilterCorpus writes every trace in filterCorpusDir and returns them by file name, without
// the extension.
func (s *StorageIntegration) writeFilterCorpus(t *testing.T) map[string]ptrace.Traces {
	entries, err := fixtures.ReadDir(filterCorpusDir)
	require.NoError(t, err)
	corpus := make(map[string]ptrace.Traces, len(entries))
	for _, entry := range entries {
		trace := loadOTLPTrace(t, filterCorpusDir+"/"+entry.Name())
		s.writeTrace(t, trace)
		corpus[strings.TrimSuffix(entry.Name(), ".json")] = trace
	}
	require.NotEmpty(t, corpus, "no trace fixtures in %s", filterCorpusDir)
	return corpus
}

// filterCorpusTraces resolves the fixture names a case expects. A name no fixture carries is a
// failure here rather than an empty trace that fails the comparison further along.
func filterCorpusTraces(t *testing.T, corpus map[string]ptrace.Traces, names []string) []ptrace.Traces {
	traces := make([]ptrace.Traces, 0, len(names))
	for _, name := range names {
		trace, ok := corpus[name]
		require.True(t, ok, "no fixture named %q in %s", name, filterCorpusDir)
		traces = append(traces, trace)
	}
	return traces
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
