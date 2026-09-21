// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// Corpus is every trace RunSpanStoreTests writes. Building it is separate from writing it, and
// writing it is separate from asserting against it, so that one Jaeger process can write the
// corpus and a second one read it back — which is what the backward-compatibility suite does with
// an older binary. Both phases share one Corpus value, so the timestamps the reader compares are
// the ones the writer wrote; loading a fixture moves its dates to a recent day, and a corpus built
// twice would move them twice.
//
// The suite writes the whole corpus once and then only reads, so no test may depend on the
// backend holding nothing else. Every dataset below therefore carries something that tells it
// apart from the rest of the corpus — its own trace ID, its own service name, or a marker
// attribute — and each assertion searches on that.
type Corpus struct {
	// Example is the fixture the service, operation, single-trace and summary assertions read.
	Example ptrace.Traces

	// Large is a trace of over 10k spans, present only when CorpusOptions asked for it.
	Large ptrace.Traces
	// Duplicates is a trace that repeats a span ID, which a backend must return as written.
	Duplicates ptrace.Traces

	// CrossService is two traces from different services that share a marker attribute, for the
	// RFC 0013 search that carries no service name.
	CrossService []ptrace.Traces

	// Queries are the search cases, and QueryTraces the traces they expect, by fixture name.
	Queries     []*QueryFixtures
	QueryTraces map[string]ptrace.Traces

	// Filter is the RFC 0005 filter corpus by fixture name. Its traces are the only ones under a
	// service named "filter-*", which is how the filter cases tell them apart from the rest.
	Filter map[string]ptrace.Traces

	// hasLarge and hasDuplicates record whether those datasets were built. A backend that has
	// excused itself from the matching assertion does not pay to write them either, which is
	// where the cost of the 10k-span trace actually is.
	hasLarge      bool
	hasDuplicates bool
}

const (
	// crossServiceMarker is the attribute the RFC 0013 search matches on.
	crossServiceMarker = "rfc0013.cross.service"

	// largeTraceTest and duplicateSpansTest name the assertions each synthetic dataset exists
	// for, so that a backend's skip list decides whether the corpus carries it.
	largeTraceTest     = "GetLargeTrace"
	duplicateSpansTest = "GetTraceWithDuplicateSpans"

	largeTraceService      = "large-trace-service"
	duplicateSpansService  = "duplicate-spans-service"
	largeTraceSpanCount    = 10008
	duplicateTraceSpans    = 200
	duplicateSpanFrequency = 20
)

// BuildCorpus assembles the corpus without writing anything, and without needing a storage
// backend: a suite that writes the corpus from one process and reads it back from another builds
// it once, outside both. suiteFixtures are the calling suite's own search cases, which are
// searched alongside the shared ones in fixtures/queries.json.
func BuildCorpus(t *testing.T, suiteFixtures []*QueryFixtures, caps capabilities.Capabilities) *Corpus {
	c := &Corpus{
		Example:     getTraceFixture(t, "example_trace"),
		QueryTraces: make(map[string]ptrace.Traces),
		Filter:      make(map[string]ptrace.Traces),
	}

	// Note: all cases include ServiceName + StartTime range.
	c.Queries = append(c.Queries, suiteFixtures...)
	c.Queries = append(c.Queries, LoadAndParseQueryTestCases(t, "fixtures/queries.json")...)

	// Each query case names only the traces it must match, never counterexamples, so every
	// trace any case names is written and each case is then answered out of the whole set.
	for _, queryCase := range c.Queries {
		for _, name := range queryCase.ExpectedFixtures {
			if _, ok := c.QueryTraces[name]; !ok {
				c.QueryTraces[name] = getTraceFixture(t, name)
			}
		}
	}

	// The large and duplicate traces are built from example_trace, so they are retagged onto a
	// service and a trace ID of their own. Sharing example_trace's identity is what made the
	// per-test purge load-bearing: their spans landed under the trace the single-trace and
	// summary assertions read.
	entries, err := fixtures.ReadDir(filterCorpusDir)
	require.NoError(t, err)
	for _, entry := range entries {
		c.Filter[strings.TrimSuffix(entry.Name(), ".json")] = loadOTLPTrace(t, filterCorpusDir+"/"+entry.Name())
	}
	require.NotEmpty(t, c.Filter, "no trace fixtures in %s", filterCorpusDir)

	c.hasLarge = !skipsTest(caps, largeTraceTest)
	if c.hasLarge {
		c.Large = buildSyntheticTrace(t, largeTraceSpanCount, 0, largeTraceService, 0xA1)
	}
	c.hasDuplicates = !skipsTest(caps, duplicateSpansTest)
	if c.hasDuplicates {
		c.Duplicates = buildSyntheticTrace(t, duplicateTraceSpans, duplicateSpanFrequency, duplicateSpansService, 0xD1)
	}
	c.CrossService = buildCrossServiceTraces()

	return c
}

// buildSyntheticTrace grows example_trace to totalCount spans under its own service and trace ID.
// Every dupFreq-th span repeats the first span's ID when dupFreq is positive.
func buildSyntheticTrace(t *testing.T, totalCount, dupFreq int, service string, traceIDByte byte) ptrace.Traces {
	source := getTraceFixture(t, "example_trace")
	sourceResource := source.ResourceSpans().At(0)
	sourceScope := sourceResource.ScopeSpans().At(0)
	sourceSpan := sourceScope.Spans().At(0)

	newResource := ptrace.NewResourceSpans()
	sourceResource.Resource().CopyTo(newResource.Resource())
	newResource.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, service)
	newScope := newResource.ScopeSpans().AppendEmpty()
	sourceScope.Scope().CopyTo(newScope.Scope())

	var traceID pcommon.TraceID
	traceID[0] = traceIDByte
	for i := 1; i < len(traceID); i++ {
		traceID[i] = byte(i)
	}

	spans := newScope.Spans()
	spans.EnsureCapacity(totalCount)
	for i := range totalCount {
		span := ptrace.NewSpan()
		sourceSpan.CopyTo(span)
		span.SetTraceID(traceID)
		switch {
		case dupFreq > 0 && i > 0 && i%dupFreq == 0:
			span.SetSpanID(sourceSpan.SpanID())
		default:
			var spanID [8]byte
			binary.BigEndian.PutUint64(spanID[:], uint64(i+1))
			span.SetSpanID(spanID)
		}
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(span.StartTimestamp().AsTime().Add(time.Second * time.Duration(i+1))))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(span.EndTimestamp().AsTime().Add(time.Second * time.Duration(i+1))))
		span.CopyTo(spans.AppendEmpty())
	}

	trace := ptrace.NewTraces()
	newResource.CopyTo(trace.ResourceSpans().AppendEmpty())
	return trace
}

// buildCrossServiceTraces are two traces of different services carrying one shared attribute, so
// that a search on the attribute alone must return both.
func buildCrossServiceTraces() []ptrace.Traces {
	traces := make([]ptrace.Traces, 0, 2)
	for i, service := range []string{"cross-service-a", "cross-service-b"} {
		trace := ptrace.NewTraces()
		rs := trace.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, service)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{byte(i + 1), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID(pcommon.SpanID{byte(i + 1), 2, 3, 4, 5, 6, 7, 8})
		span.SetName("cross-service-op")
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Millisecond)))
		span.Attributes().PutStr(crossServiceMarker, "yes")
		traces = append(traces, trace)
	}
	return traces
}

// All returns every trace in the corpus, in the order the write phase writes them.
func (c *Corpus) All() []ptrace.Traces {
	all := []ptrace.Traces{c.Example}
	if c.hasLarge {
		all = append(all, c.Large)
	}
	if c.hasDuplicates {
		all = append(all, c.Duplicates)
	}
	all = append(all, c.CrossService...)
	for _, name := range slices.Sorted(maps(c.QueryTraces)) {
		all = append(all, c.QueryTraces[name])
	}
	for _, name := range slices.Sorted(maps(c.Filter)) {
		all = append(all, c.Filter[name])
	}
	return all
}

// Services are the service names the corpus writes, sorted and deduplicated. GetServices asserts
// against this rather than a hardcoded list, so the assertion stays exact once the backend holds
// the whole corpus instead of one trace.
func (c *Corpus) Services() []string {
	seen := make(map[string]struct{})
	for _, trace := range c.All() {
		for i := 0; i < trace.ResourceSpans().Len(); i++ {
			attrs := trace.ResourceSpans().At(i).Resource().Attributes()
			if name, ok := attrs.Get(otelsemconv.ServiceNameKey); ok {
				seen[name.Str()] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// AbsentTraceID is an ID no trace in the corpus carries, for the assertion that a lookup of an
// unknown trace comes back empty. It is computed rather than hardcoded because three of the query
// fixtures do carry the all-but-last-byte-zero ID that assertion used to fabricate, which the
// per-test purge hid: the lookup ran when only one trace was in the backend.
func (c *Corpus) AbsentTraceID(t *testing.T) pcommon.TraceID {
	present := make(map[pcommon.TraceID]struct{})
	for _, trace := range c.All() {
		for _, span := range jptrace.SpanIter(trace) {
			present[span.TraceID()] = struct{}{}
		}
	}
	for b := range 256 {
		var candidate pcommon.TraceID
		for i := range candidate {
			candidate[i] = byte(b)
		}
		if _, taken := present[candidate]; !taken {
			return candidate
		}
	}
	require.Fail(t, "no absent trace ID left to probe with")
	return pcommon.TraceID{}
}

// QueryExpectations resolves each search case to the traces it must return. A case naming a
// fixture the corpus does not hold fails here rather than as an empty comparison further along.
func (c *Corpus) QueryExpectations(t *testing.T) [][]ptrace.Traces {
	expectations := make([][]ptrace.Traces, 0, len(c.Queries))
	for _, queryCase := range c.Queries {
		var expected []ptrace.Traces
		for _, name := range queryCase.ExpectedFixtures {
			trace, ok := c.QueryTraces[name]
			require.True(t, ok, "no fixture named %q in the corpus", name)
			expected = append(expected, trace)
		}
		expectations = append(expectations, expected)
	}
	return expectations
}

// WriteCorpus writes the whole corpus and performs no reads, so that it can run as a phase of its
// own against a Jaeger binary that is stopped before anything reads the data back.
func (s *StorageIntegration) WriteCorpus(t *testing.T) {
	s.requireCorpus(t)
	traces := s.Corpus.All()
	t.Logf("Writing corpus: %d traces, %d spans", len(traces), spanCount(traces))
	for _, trace := range traces {
		s.writeTrace(t, trace)
	}
}

// maps returns the keys of m. (slices.Sorted wants an iterator; this keeps the call sites short.)
func maps[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// skipsTest reports whether the backend has excused itself from the named test, matching a skip
// list entry the same way skipIfNeeded does — as a substring of the test's name.
func skipsTest(caps capabilities.Capabilities, name string) bool {
	for _, pat := range caps.SkipList() {
		if strings.Contains(name, pat) {
			return true
		}
	}
	return false
}
