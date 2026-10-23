// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

const completeSearchService = "complete-trace-search"

func buildCompleteSearchTrace() ptrace.Traces {
	trace := ptrace.NewTraces()
	// Keep shared fixtures within the microsecond precision supported by all backends.
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	for i, fixture := range []struct {
		service string
		offset  time.Duration
	}{
		{service: "complete-trace-other", offset: 123456 * time.Microsecond},
		{service: completeSearchService, offset: time.Second + 123456*time.Microsecond},
		{service: "complete-trace-other", offset: 3*time.Second + 987654*time.Microsecond},
	} {
		rs := trace.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, fixture.service)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{0xC1, 0xF1})
		// Avoid span ID collisions when ingestion batches this trace with the rest of the corpus.
		span.SetSpanID(pcommon.SpanID{0xC1, 0xF1, byte(i + 1)})
		span.SetName("complete-trace-operation")
		start := base.Add(fixture.offset)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Microsecond)))
	}
	return trace
}

func (s *StorageIntegration) testFindCompleteTraces(t *testing.T) {
	s.skipIfNeeded(t)
	expected := s.Corpus.CompleteSearch
	var start time.Time
	for _, rs := range expected.ResourceSpans().All() {
		if service, _ := rs.Resource().Attributes().Get(otelsemconv.ServiceNameKey); service.Str() == completeSearchService {
			start = rs.ScopeSpans().At(0).Spans().At(0).StartTimestamp().AsTime().Truncate(time.Second)
		}
	}
	require.False(t, start.IsZero(), "the complete-search trace has no span of %s", completeSearchService)
	query := &tracestore.TraceQueryParams{
		ServiceName:  completeSearchService,
		Attributes:   pcommon.NewMap(),
		StartTimeMin: start,
		StartTimeMax: start.Add(time.Second),
		SearchDepth:  1,
	}
	actual := s.findTracesByQuery(t, query, []ptrace.Traces{expected})
	for _, trace := range actual {
		CompareTraces(t, expected, trace)
	}
}
