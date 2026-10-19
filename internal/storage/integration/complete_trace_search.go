// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func buildCompleteSearchTrace() ptrace.Traces {
	trace := ptrace.NewTraces()
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	for i, fixture := range []struct {
		service string
		offset  time.Duration
	}{
		{service: "complete-trace-other", offset: 123456789 * time.Nanosecond},
		{service: "complete-trace-search", offset: time.Second + 123456789*time.Nanosecond},
		{service: "complete-trace-other", offset: 3*time.Second + 987654321*time.Nanosecond},
	} {
		rs := trace.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, fixture.service)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{0xC1, 0xF1})
		span.SetSpanID(pcommon.SpanID{byte(i + 1)})
		span.SetName("complete-trace-operation")
		start := base.Add(fixture.offset)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Nanosecond)))
	}
	return trace
}

func (s *StorageIntegration) testFindCompleteTraces(t *testing.T) {
	s.skipIfNeeded(t)
	expected := s.Corpus.QueryTraces["complete_search"]
	matched := expected.ResourceSpans().At(1).ScopeSpans().At(0).Spans().At(0)
	start := matched.StartTimestamp().AsTime().Truncate(time.Second)
	query := &tracestore.TraceQueryParams{
		ServiceName:  "complete-trace-search",
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
