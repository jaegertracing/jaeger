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
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// maxSearchDepth is the tightest ceiling the suite's backends put on SearchDepth: the memory
// store refuses a depth above its MaxTraces, which every memory-backed harness sets to 10000,
// and ClickHouse refuses one above its own MaxSearchDepth, which defaults to the same number.
const maxSearchDepth = 10_000

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
