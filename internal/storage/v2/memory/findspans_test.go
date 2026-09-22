// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// spanNames collects every span's name from a ptrace.Traces, across however
// many ResourceSpans/ScopeSpans it holds, for asserting on FindSpans results
// without caring how the store chose to group them.
func spanNames(td ptrace.Traces) []string {
	var names []string
	for _, rs := range td.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				names = append(names, span.Name())
			}
		}
	}
	return names
}

func writeTwoTraceStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	traces := ptrace.NewTraces()

	rs1 := traces.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "frontend")
	ss1 := rs1.ScopeSpans().AppendEmpty()
	span1 := ss1.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID{1})
	span1.SetSpanID(pcommon.SpanID{1})
	span1.SetName("GET /")
	span1.SetStartTimestamp(pcommon.NewTimestampFromTime(base))
	span1.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(10 * time.Millisecond)))

	span2 := ss1.Spans().AppendEmpty()
	span2.SetTraceID(pcommon.TraceID{1})
	span2.SetSpanID(pcommon.SpanID{2})
	span2.SetParentSpanID(pcommon.SpanID{1})
	span2.SetName("call-backend")
	span2.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(1 * time.Millisecond)))
	span2.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(9 * time.Millisecond)))

	rs2 := traces.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "backend")
	ss2 := rs2.ScopeSpans().AppendEmpty()
	span3 := ss2.Spans().AppendEmpty()
	span3.SetTraceID(pcommon.TraceID{2})
	span3.SetSpanID(pcommon.SpanID{3})
	span3.SetName("handle-request")
	span3.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(24 * time.Hour)))
	span3.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(24*time.Hour + 5*time.Millisecond)))

	require.NoError(t, store.WriteTraces(context.Background(), traces))
	return store, base
}

func TestFindSpans_MatchesAcrossTraces(t *testing.T) {
	store, _ := writeTwoTraceStore(t)

	filter := &expression.Call{
		Op: expression.OpRegex,
		Args: []expression.Expression{
			&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
			&expression.StringValue{Value: "^(GET /|handle-request)$"},
		},
	}

	var results []ptrace.Traces
	for chunk, err := range store.FindSpans(context.Background(), tracestore.SpanQueryParams{Filter: filter}) {
		require.NoError(t, err)
		results = append(results, chunk.Results)
		assert.Empty(t, chunk.NextPageToken)
	}
	require.Len(t, results, 1)

	names := spanNames(results[0])
	assert.ElementsMatch(t, []string{"GET /", "handle-request"}, names,
		"a span-search result holds exactly the matching spans, from however many different traces")
}

func TestFindSpans_TimeRangeBound(t *testing.T) {
	store, base := writeTwoTraceStore(t)

	var results []ptrace.Traces
	for chunk, err := range store.FindSpans(context.Background(), tracestore.SpanQueryParams{
		StartTimeMin: base.Add(-time.Minute),
		StartTimeMax: base.Add(time.Minute),
	}) {
		require.NoError(t, err)
		results = append(results, chunk.Results)
	}
	require.Len(t, results, 1)

	// Only the two spans on the first day fall in the bound; handle-request,
	// 24h later, does not.
	assert.ElementsMatch(t, []string{"GET /", "call-backend"}, spanNames(results[0]))
}

func TestFindSpans_NilFilterReturnsEverySpan(t *testing.T) {
	store, _ := writeTwoTraceStore(t)

	var results []ptrace.Traces
	for chunk, err := range store.FindSpans(context.Background(), tracestore.SpanQueryParams{}) {
		require.NoError(t, err)
		results = append(results, chunk.Results)
	}
	require.Len(t, results, 1)
	assert.ElementsMatch(t, []string{"GET /", "call-backend", "handle-request"}, spanNames(results[0]))
}

func TestFindSpans_NoMatchesYieldsEmptyChunk(t *testing.T) {
	store, _ := writeTwoTraceStore(t)

	filter := &expression.Call{
		Op: expression.OpEq,
		Args: []expression.Expression{
			&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
			&expression.StringValue{Value: "nonexistent"},
		},
	}

	var results []ptrace.Traces
	for chunk, err := range store.FindSpans(context.Background(), tracestore.SpanQueryParams{Filter: filter}) {
		require.NoError(t, err)
		results = append(results, chunk.Results)
	}
	require.Len(t, results, 1)
	assert.Empty(t, spanNames(results[0]))
}

// TestFindSpans_ResultIsIndependentOfStore proves the clone contract FindSpans
// shares with GetTraces/FindTraces: mutating what a caller received must not
// rewrite what the store holds for the next caller.
func TestFindSpans_ResultIsIndependentOfStore(t *testing.T) {
	store, _ := writeTwoTraceStore(t)

	var first ptrace.Traces
	for chunk, err := range store.FindSpans(context.Background(), tracestore.SpanQueryParams{}) {
		require.NoError(t, err)
		first = chunk.Results
		break
	}
	require.Positive(t, first.ResourceSpans().Len())
	first.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).SetName("mutated")

	var second ptrace.Traces
	for chunk, err := range store.FindSpans(context.Background(), tracestore.SpanQueryParams{}) {
		require.NoError(t, err)
		second = chunk.Results
		break
	}
	assert.NotContains(t, spanNames(second), "mutated")
}

func TestFindSpans_UnsupportedWithoutSpanSearchCapability(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	caps, err := store.SearchCapabilities(context.Background())
	require.NoError(t, err)
	assert.True(t, caps.SpanSearch)
	require.NotNil(t, caps.Filter)
	assert.ElementsMatch(t, expression.Levels(), caps.Filter.Levels)
	assert.ElementsMatch(t, expression.Operators(), caps.Filter.Operators)
}

// TestFindTraces_StructuredFilter pins that a TraceQueryParams.Filter reaches
// the same evaluator FindSpans uses: FindTraces looks for a trace with at
// least one matching span, not the matching spans themselves.
func TestFindTraces_StructuredFilter(t *testing.T) {
	store, _ := writeTwoTraceStore(t)

	filter := &expression.Call{
		Op: expression.OpEq,
		Args: []expression.Expression{
			&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
			&expression.StringValue{Value: "call-backend"},
		},
	}

	var traces []ptrace.Traces
	for batch, err := range store.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Filter:      filter,
		SearchDepth: 10,
	}) {
		require.NoError(t, err)
		traces = append(traces, batch...)
	}
	require.Len(t, traces, 1, "the whole trace containing the matching span, not just that span")

	names := spanNames(traces[0])
	assert.ElementsMatch(t, []string{"GET /", "call-backend"}, names,
		"FindTraces returns every span of a matched trace, unlike FindSpans")
}

func TestFindTraces_StructuredFilterNoMatch(t *testing.T) {
	store, _ := writeTwoTraceStore(t)

	filter := &expression.Call{
		Op: expression.OpEq,
		Args: []expression.Expression{
			&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
			&expression.StringValue{Value: "nonexistent"},
		},
	}

	var traces []ptrace.Traces
	for batch, err := range store.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Filter:      filter,
		SearchDepth: 10,
	}) {
		require.NoError(t, err)
		traces = append(traces, batch...)
	}
	assert.Empty(t, traces)
}
