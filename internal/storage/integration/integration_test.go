// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// maxSearchDepth is the tightest ceiling the suite's backends put on SearchDepth: the memory
// store refuses a depth above its MaxTraces, which every memory-backed harness sets to 10000,
// and ClickHouse refuses one above its own MaxSearchDepth, which defaults to the same number.
const maxSearchDepth uint32 = 10_000

// TestGetServicesUnexpectedServiceDiagnostic drives testGetServices into the branch that only
// runs when the backend reports more services than the corpus wrote, which is where the
// diagnostic search lives. That search is a Reader call like any other, so it owes the Reader
// contract: Attributes initialized with pcommon.NewMap(), because readers dereference it before
// they do anything else, and an explicit SearchDepth, because the memory store rejects a
// non-positive one and so never reached the dereference that panicked elsewhere.
func TestGetServicesUnexpectedServiceDiagnostic(t *testing.T) {
	const (
		corpusService     = "corpus-service"
		unexpectedService = "unexpected-service"
	)

	trace := ptrace.NewTraces()
	resource := trace.ResourceSpans().AppendEmpty()
	resource.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, corpusService)
	resource.ScopeSpans().AppendEmpty().Spans().AppendEmpty()

	reader := new(tracestoremocks.Reader)
	// The first read reports a service the corpus never wrote, which is the only way into the
	// diagnostic; every later read agrees with the corpus so the assertion settles immediately
	// instead of spending the full waitForCondition budget.
	reader.EXPECT().GetServices(mock.Anything).
		Return([]string{corpusService, unexpectedService}, nil).Once()
	reader.EXPECT().GetServices(mock.Anything).
		Return([]string{corpusService}, nil)

	var queries []tracestore.TraceQueryParams
	reader.EXPECT().FindTraces(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			// Reading Attributes is the first thing the Cassandra and Elasticsearch readers do
			// with a query, and a zero-value pcommon.Map panics there rather than reading as
			// empty, so this stands in for them.
			require.Equal(t, 0, query.Attributes.Len())
			queries = append(queries, query)
			return func(func([]ptrace.Traces, error) bool) {}
		},
	)

	s := &StorageIntegration{
		TraceReader: reader,
		Corpus:      &Corpus{Example: trace},
	}
	s.testGetServices(t)

	require.Len(t, queries, 2, "the diagnostic searches once per reported service")
	for _, query := range queries {
		assert.Positive(t, query.SearchDepth, "the memory store rejects a non-positive search depth")
		assert.LessOrEqual(t, query.SearchDepth, maxSearchDepth, "the search depth must stay under every backend's ceiling")
	}
	assert.Equal(t, []string{corpusService, unexpectedService},
		[]string{queries[0].ServiceName, queries[1].ServiceName})
}

func TestQueryToTraceQueryParamsPreservesSearchDepth(t *testing.T) {
	query := Query{NumTraces: tracestore.MaxSearchDepth + 1}
	assert.Equal(t, tracestore.MaxSearchDepth+1, query.ToTraceQueryParams(t).SearchDepth)
}

// TestQueryFixturesHaveDistinctStartTimesPerService guards the shared search fixtures against two
// traces of one service whose spans start in the same microsecond. The Cassandra
// service_operation_index keys a row by (service_name, operation_name, start_time) and the
// service_name_index by (service_name, bucket, start_time), neither with a trace_id, so such spans
// overwrite each other: always in the first index, and in the second whenever their hash lands in
// the same bucket. The hash moves with the date-shifted start time, so a search case that expects
// both traces fails on some days and passes on others.
func TestQueryFixturesHaveDistinctStartTimesPerService(t *testing.T) {
	type key struct {
		service string
		start   uint64
	}
	seen := make(map[key]pcommon.TraceID)
	for _, queryCase := range LoadAndParseQueryTestCases(t, "fixtures/queries.json") {
		for _, name := range queryCase.ExpectedFixtures {
			for pos, span := range jptrace.SpanIter(getTraceFixture(t, name)) {
				service, _ := pos.Resource.Resource().Attributes().Get(otelsemconv.ServiceNameKey)
				k := key{service: service.Str(), start: uint64(span.StartTimestamp()) / 1000}
				if other, ok := seen[k]; ok && other != span.TraceID() {
					assert.Failf(t, "same service and start microsecond",
						"traces %s and %s of %s both have a span starting at %d (fixture %s)",
						other, span.TraceID(), k.service, span.StartTimestamp(), name)
				}
				seen[k] = span.TraceID()
			}
		}
	}
}
