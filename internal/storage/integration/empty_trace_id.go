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

const emptyTraceIDService = "empty-trace-id-service"

func buildEmptyTraceIDTraces() []ptrace.Traces {
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	var traces []ptrace.Traces
	for i, id := range []pcommon.TraceID{{0xF1, 1}, {}, {0xF1, 2}} {
		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, emptyTraceIDService)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(id)
		span.SetSpanID(pcommon.SpanID{0xF1, byte(i + 1)})
		span.SetName("valid-operation")
		if id.IsEmpty() {
			span.SetName("invalid-operation")
		}
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Millisecond)))
		traces = append(traces, td)
	}
	return traces
}

func (s *StorageIntegration) testFindTracesAfterEmptyTraceID(t *testing.T) {
	s.skipIfNeeded(t)
	expected := []ptrace.Traces{s.Corpus.EmptyTraceID[0], s.Corpus.EmptyTraceID[2]}
	span := expected[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	query := &tracestore.TraceQueryParams{
		ServiceName:   emptyTraceIDService,
		OperationName: "valid-operation",
		Attributes:    pcommon.NewMap(),
		StartTimeMin:  span.StartTimestamp().AsTime().Add(-time.Second),
		StartTimeMax:  span.EndTimestamp().AsTime().Add(time.Second),
		SearchDepth:   10,
	}
	// Every backend must keep valid traces queryable even when a malformed trace
	// was also ingested. The memory store's ring invariants are tested in its package.
	actual := s.findTracesByQuery(t, query, expected)
	CompareTraceSlices(t, expected, actual)
}
