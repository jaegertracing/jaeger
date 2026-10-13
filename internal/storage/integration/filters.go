// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"slices"
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
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
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

// filterTestCases covers the RFC 0005 filter model across the backends that evaluate it natively:
// the five OTLP attribute levels, the built-in fields each backend routes to a column or field of
// its own, the operators and typed constants they declare, and boolean composition over the lot.
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
			caption:  "a scope-level attribute matches the instrumentation scope's attributes only",
			filter:   p.Scope().Attr("zone").Eq("us-east"),
			expected: []string{"worker"},
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
			caption:  "a link-level attribute matches an attribute of one of the span's links",
			filter:   p.Link().Attr("zone").Eq("eu-west"),
			expected: []string{"worker"},
		},
		{
			// The span-or-resource default of RFC 0005 §5.1, asserted by what it leaves out: the
			// `search` trace carries `zone` on an event and the `worker` trace on its scope and on
			// a link, and neither is reached.
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
			caption:  "the span kind",
			filter:   p.Span().Kind.Eq("consumer"),
			expected: []string{"worker"},
		},
		{
			caption:  "the span status",
			filter:   p.Span().Status.Eq("error"),
			expected: []string{"worker"},
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
			// `some` quantifies a predicate over the events of one span (RFC 0005 §5.5), so a
			// backend that flattens events into the span's own fields cannot evaluate it.
			caption:  "a predicate quantified over the span's events",
			filter:   p.Some(p.Event(), p.Event().Name.Eq("exception")),
			expected: []string{"cart_post"},
		},
		{
			caption:  "an exact duration",
			filter:   p.Span().Duration.Eq(40 * time.Millisecond),
			expected: []string{"checkout"},
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
			caption:  "an untyped constant matches an attribute stored as a number",
			filter:   p.Span().Attr("retry.count").Eq(11),
			expected: []string{"cart_post"},
		},
		{
			// `cart_get` stores `retry.count` as the integer 9 and `search` as the string "09". A
			// constant declaring the string type (RFC 0005 §5.4) is compared as text, so it
			// matches `search` only; the same value sent untyped would be read as the number 9
			// and match `cart_get` as well. The value is zero-padded so that a backend comparing
			// this attribute lexicographically does not also count it as greater than "10" in
			// "an ordered comparison on an attribute" below.
			caption:  "a string-typed constant leaves out an attribute stored as a number",
			filter:   p.Span().Attr("retry.count").Eq(p.String("09")),
			expected: []string{"search"},
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
//
// Every case is therefore a conjunction of the scope below and the predicate it is testing. That
// costs some isolation — a fault in how the backend lowers a conjunction fails the whole battery
// rather than one case — but a conjunction that fails everywhere at once is easy to read, and it
// is what lets these cases run against a corpus the suite wrote for every other assertion too.
func (s *StorageIntegration) testFindTracesWithFilter(t *testing.T) {
	s.skipIfNeeded(t)

	corpus := s.Corpus.Filter
	start, end := filterCorpusTimeRange(corpus)
	var p builder.Predicate
	// The suite's corpus holds far more than these fixtures, and a case's time range covers all of
	// it, so every case below is scoped to the services only these fixtures use. The scope is
	// derived from the corpus rather than listed here, so a fixture added to filterCorpusDir is
	// searched without being named anywhere, and it is a disjunction of equalities rather than In
	// so every backend that evaluates a boolean filter can serve it.
	scope := filterCorpusScope(p, corpus)

	// A deployment that will not serve a filter at all refuses every case below, and a search that
	// comes back with an error is retried for a minute and a half before it is called a failure —
	// which for a whole battery is half an hour of a CI run spent on one misconfiguration. So the
	// first filter goes straight to the reader, whose error says what is actually wrong.
	s.requireFilterIsServed(t, filterQuery(t, scope, start, end))

	// RFC 0005 §7 promises that a backend either evaluates a predicate or refuses it, and never
	// answers a predicate it cannot evaluate with a wider result set. Which of the two a case
	// expects follows from the levels and operators the reader declares (ADR-013): a declared
	// predicate must return the traces the corpus holds for it, and an undeclared one must be
	// refused. The expectation is derived from the declaration and not from the suite's
	// Capabilities, so a reader that evaluates more than it declares fails the refusal, and one
	// that declares more than it evaluates fails the search; neither direction passes vacuously.
	declared := s.declaredFilterCapabilities(t)
	for _, testCase := range filterTestCases(p) {
		t.Run(testCase.caption, func(t *testing.T) {
			s.skipIfNeeded(t)
			query := filterQuery(t, p.And(scope, testCase.filter), start, end)
			if declared.EnsureSupported(testCase.filter) != nil {
				_, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
					s.TraceReader.FindTraces(context.Background(), *query),
				))
				requireRefusal(t, err)
				return
			}
			expected := filterCorpusTraces(t, corpus, testCase.expected)
			actual := s.findTracesByQuery(t, query, expected)
			CompareTraceSlices(t, expected, actual)
		})
	}

	// FilterCapabilities declares levels and operators, not built-in fields, so whether the trace
	// state is indexed is declared by the deployment through Capabilities.TraceStateRefused
	// instead, and this case asserts the declared outcome. `worker` carries a trace state with
	// another value, so a backend that matched the field's presence rather than its value would
	// answer with both traces and fail.
	t.Run("the trace state", func(t *testing.T) {
		s.skipIfNeeded(t)
		query := filterQuery(t, p.And(scope, p.Span().TraceState.Eq("congo=t61rcWkgMzE")), start, end)
		if s.Capabilities.TraceStateRefused() {
			_, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
				s.TraceReader.FindTraces(context.Background(), *query),
			))
			requireRefusal(t, err)
			require.ErrorContains(t, err, "traceState")
			return
		}
		expected := filterCorpusTraces(t, corpus, []string{"search"})
		CompareTraceSlices(t, expected, s.findTracesByQuery(t, query, expected))
	})

	// An ordered comparison on an attribute needs the value stored as a number, which depends on
	// the index mapping rather than on anything the reader declares: the operator and the level
	// are both declared, and only the pairing can be unservable. So the deployment names the
	// outcome it produces through Capabilities.AttributeComparison, and this case asserts it.
	//
	// The corpus carries retry.count as 9 and 11, and the bound is 10, because those are the
	// numbers that tell a numeric comparison from a lexicographic one. Numerically only 11 is
	// greater; as text both are, since "9" sorts after "10". So a backend that ranged over the
	// keyword would answer with cart_get as well — a superset, which is the failure this case
	// exists to catch, and one that cannot be mistaken for the writes not having landed.
	t.Run("an ordered comparison on an attribute", func(t *testing.T) {
		s.skipIfNeeded(t)
		query := filterQuery(t, p.And(scope, p.Span().Attr("retry.count").Gt(10)), start, end)
		switch outcome := s.Capabilities.AttributeComparison(); outcome {
		case capabilities.AttributeComparisonNumeric:
			expected := filterCorpusTraces(t, corpus, []string{"cart_post"})
			CompareTraceSlices(t, expected, s.findTracesByQuery(t, query, expected))
		case capabilities.AttributeComparisonRefused:
			// The reader refuses inside its lowering rather than at the capability edge, so the
			// message is the reader's own; what every reader's message has in common is the
			// attribute it refuses to compare.
			_, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
				s.TraceReader.FindTraces(context.Background(), *query),
			))
			requireRefusal(t, err)
			require.ErrorContains(t, err, "retry.count")
		case capabilities.AttributeComparisonMissesOlderIndices:
			// The reader ranges over the numeric sub-field, but the corpus was written into indices
			// created before the mapping was turned on, so the range finds nothing there. RFC 0005
			// §7 admits this as a data gap rather than a refusal, and this case pins that the
			// reader neither errors nor falls back to comparing the keyword, which would answer
			// with cart_get as well.
			//
			// The scope alone must find the whole corpus first, so that the empty answer below is
			// the range's doing and not an index that has not caught up yet.
			names := make([]string, 0, len(corpus))
			for name := range corpus {
				names = append(names, name)
			}
			s.findTracesByQuery(t, filterQuery(t, scope, start, end), filterCorpusTraces(t, corpus, names))
			// The reader is asked directly rather than through findTracesByQuery, which retries an
			// error for the whole wait: a refusal here is wrong at once and should say so.
			actual, err := jiter.CollectWithErrors(jptrace.AggregateTraces(
				s.TraceReader.FindTraces(context.Background(), *query),
			))
			require.NoError(t, err, "the range must be evaluated, not refused")
			require.Empty(t, actual)
		default:
			t.Fatalf("unknown attribute comparison outcome %d", outcome)
		}
	})
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
		filter := filterQuery(t, p.And(
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

// requireRefusal asserts that a search was refused for a capability the backend lacks, rather than
// failing in some other way or answering with a wider result set. The refusal's own sentinel does
// not survive the gRPC hop the e2e suite reads through, but its family does: the api_v3 edge
// answers it with Unimplemented, which the e2e reader restores to errors.ErrUnsupported (ADR-013).
// A case whose refusal the declaration does not predict checks the message as well for what was
// refused, because a refusal of the wrong thing would pass the family check alone.
func requireRefusal(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, errors.ErrUnsupported, "the search must be refused rather than answered with a wider result set")
}

// declaredFilterCapabilities is what the reader under test declares it evaluates of a filter. A
// reader that declares nothing is held to refusing every level and operator.
func (s *StorageIntegration) declaredFilterCapabilities(t *testing.T) tracestore.FilterCapabilities {
	t.Helper()
	caps, err := s.TraceReader.SearchCapabilities(context.Background())
	require.NoError(t, err)
	if caps.Filter == nil {
		return tracestore.FilterCapabilities{}
	}
	return *caps.Filter
}

// filterQuery is a search whose only predicate is the filter. The filter is finalized here so a
// direct suite hands its Reader the same resolved AST the query service and the remote-storage
// server hand it in production (RFC 0005 §7). The attributes map is empty rather than unset
// because a Reader is entitled to read it (it "must be initialized with pcommon.NewMap() before
// use"), and a filter must arrive alone: a query carrying one beside a service name, an operation
// name, a tag or a duration bound is refused.
func filterQuery(t *testing.T, filter *expression.Call, start, end time.Time) *tracestore.TraceQueryParams {
	t.Helper()
	finalized, err := tracestore.FinalizeFilter(filter)
	require.NoError(t, err)
	return &tracestore.TraceQueryParams{
		Filter:       finalized,
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

// filterCorpusScope builds the disjunction of service-name equalities that restricts a search to
// the services the filter fixtures use. Only these fixtures use a "filter-" prefixed service,
// which is what lets a case be scoped to them.
func filterCorpusScope(p builder.Predicate, corpus map[string]ptrace.Traces) *expression.Call {
	seen := make(map[string]struct{})
	for _, trace := range corpus {
		for i := 0; i < trace.ResourceSpans().Len(); i++ {
			if name, ok := trace.ResourceSpans().At(i).Resource().Attributes().Get(otelsemconv.ServiceNameKey); ok {
				seen[name.Str()] = struct{}{}
			}
		}
	}
	clauses := make([]*expression.Call, 0, len(seen))
	for _, name := range slices.Sorted(maps(seen)) {
		clauses = append(clauses, p.Resource().Service.Eq(name))
	}
	return p.Or(clauses...)
}

// writeFilterCorpus writes every trace in filterCorpusDir and returns them by file name, without
// the extension. Only RunFilterRewriteTest uses it: that test runs outside the suite's corpus,
// because it compares the two forms of one query against each other rather than against fixtures.
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
