// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

// These tests follow the procedure written in skills/analyze-critical-path/SKILL.md
// step by step against traces where the naive answer and the procedure's answer
// differ, so they check what the skill concludes rather than only that it is
// servable. The expected answers are derived from the span timestamps, not chosen
// by hand.

const skillTraceID = "00000000000000000000000000000001"

// skillTraceBase anchors the fixtures at a real wall-clock time (2026-01-01T00:00:00Z).
// Offsets in the assertions are relative to trace start, so the base does not
// appear in them; it only keeps the spans off timestamp zero.
const skillTraceBase = uint64(1_767_225_600_000)

// cpSpan is one span of a hand-built trace, timed in milliseconds from trace start.
type cpSpan struct {
	id      byte
	parent  byte // 0 means root
	name    string
	startMs uint64
	endMs   uint64
}

func buildSkillTrace(spans []cpSpan) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout")
	ss := rs.ScopeSpans().AppendEmpty()
	for _, s := range spans {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
		span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, s.id})
		if s.parent != 0 {
			span.SetParentSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, s.parent})
		}
		span.SetName(s.name)
		span.SetStartTimestamp(pcommon.Timestamp((skillTraceBase + s.startMs) * 1_000_000))
		span.SetEndTimestamp(pcommon.Timestamp((skillTraceBase + s.endMs) * 1_000_000))
	}
	return traces
}

func spanIDOf(id byte) string {
	return pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, id}).String()
}

func criticalPathOf(t *testing.T, traces ptrace.Traces) types.GetCriticalPathOutput {
	t.Helper()
	h := &getCriticalPathHandler{queryService: &mockGetCriticalPathQueryService{traces: []ptrace.Traces{traces}}}
	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.GetCriticalPathInput{TraceID: skillTraceID})
	require.NoError(t, err)
	return out
}

func skillTopologyOf(t *testing.T, traces ptrace.Traces) types.GetTraceTopologyOutput {
	t.Helper()
	h := &getTraceTopologyHandler{queryService: newMockYieldingTraces(traces), maxSpanDetailsPerRequest: 100}
	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.GetTraceTopologyInput{TraceID: skillTraceID})
	require.NoError(t, err)
	return out
}

// dominantSegment is step 2 of the skill: rank segments by self_time_us and take
// the top one.
func dominantSegment(out types.GetCriticalPathOutput) types.CriticalPathSegment {
	var top types.CriticalPathSegment
	for _, seg := range out.Segments {
		if seg.SelfTimeUs > top.SelfTimeUs {
			top = seg
		}
	}
	return top
}

// hasDescendants is step 3 of the skill: does any other span list this span_id in
// its ancestry path?
func hasDescendants(topo types.GetTraceTopologyOutput, spanID string) bool {
	for _, s := range topo.Spans {
		if strings.Contains(s.Path, spanID+"/") {
			return true
		}
	}
	return false
}

// longestChild is the naive answer the skill exists to correct: blame the child
// span with the longest duration.
func longestChild(topo types.GetTraceTopologyOutput) types.TopologySpan {
	var top types.TopologySpan
	for _, s := range topo.Spans {
		if strings.Contains(s.Path, "/") && s.DurationUs > top.DurationUs {
			top = s
		}
	}
	return top
}

func TestAnalyzeCriticalPathSkill_ParentGapBeatsLongestChild(t *testing.T) {
	// Root runs 0-200ms. Child A (db.query) runs 10-60ms, child B (http.call)
	// runs 150-190ms. Nothing is instrumented between 60ms and 150ms, so the
	// critical path carries a 90ms root segment there, larger than either child.
	// Naive answer: db.query, the longest child at 50ms. Procedure's answer: the
	// root, classified as a parent scheduling or un-instrumented gap.
	traces := buildSkillTrace([]cpSpan{
		{id: 1, name: "POST /checkout", startMs: 0, endMs: 200},
		{id: 2, parent: 1, name: "db.query", startMs: 10, endMs: 60},
		{id: 3, parent: 1, name: "http.call", startMs: 150, endMs: 190},
	})
	cp := criticalPathOf(t, traces)
	topo := skillTopologyOf(t, traces)

	// Step 1: durations come straight from the timestamps.
	assert.Equal(t, uint64(200_000), cp.TotalDurationUs)
	assert.Equal(t, uint64(200_000), cp.CriticalPathDurationUs, "a single-service trace with serial children has no off-path time")

	// Step 2: the dominant segment is the root's 60-150ms gap, 90ms, above the 30% threshold.
	top := dominantSegment(cp)
	assert.Equal(t, spanIDOf(1), top.SpanID)
	assert.Equal(t, "POST /checkout", top.SpanName)
	assert.Equal(t, uint64(90_000), top.SelfTimeUs)
	assert.Equal(t, uint64(60_000), top.StartOffsetUs)
	assert.Equal(t, uint64(150_000), top.EndOffsetUs)
	assert.Greater(t, top.SelfTimeUs*100/cp.CriticalPathDurationUs, uint64(30))

	// Step 3: the root has children listing it in their path, so this is a
	// parent scheduling or un-instrumented gap, not a leaf bottleneck.
	assert.True(t, hasDescendants(topo, top.SpanID))

	// The naive answer differs, which is what makes the fixture worth having.
	naive := longestChild(topo)
	assert.Equal(t, "db.query", naive.SpanName)
	assert.NotEqual(t, top.SpanName, naive.SpanName)
}

func TestAnalyzeCriticalPathSkill_LeafBottleneck(t *testing.T) {
	// Root runs 0-100ms and its only child (db.query) runs 5-95ms with no
	// children of its own. Naive answer: the root, because it has the longest
	// duration. Procedure's answer: db.query, a 90ms leaf execution bottleneck.
	traces := buildSkillTrace([]cpSpan{
		{id: 1, name: "GET /orders", startMs: 0, endMs: 100},
		{id: 2, parent: 1, name: "db.query", startMs: 5, endMs: 95},
	})
	cp := criticalPathOf(t, traces)
	topo := skillTopologyOf(t, traces)

	top := dominantSegment(cp)
	assert.Equal(t, spanIDOf(2), top.SpanID)
	assert.Equal(t, "db.query", top.SpanName)
	assert.Equal(t, uint64(90_000), top.SelfTimeUs)
	assert.Greater(t, top.SelfTimeUs*100/cp.CriticalPathDurationUs, uint64(30))

	// No span lists db.query in its ancestry, so it is a leaf execution bottleneck.
	assert.False(t, hasDescendants(topo, top.SpanID))

	// The longest span by duration is the root, which the procedure does not blame.
	var longest types.TopologySpan
	for _, s := range topo.Spans {
		if s.DurationUs > longest.DurationUs {
			longest = s
		}
	}
	assert.Equal(t, "GET /orders", longest.SpanName)
	assert.NotEqual(t, top.SpanName, longest.SpanName)
}

func TestAnalyzeCriticalPathSkill_ParallelBranchOffPath(t *testing.T) {
	// Gotcha "Parallel branches": child A (cache.get) runs 10-30ms and child B
	// (db.query) runs 10-90ms concurrently. A finishes inside B's window, so it
	// contributes nothing to the critical path even though it ran. The dominant
	// segment is B, and A must not appear as a segment at all.
	traces := buildSkillTrace([]cpSpan{
		{id: 1, name: "GET /cart", startMs: 0, endMs: 100},
		{id: 2, parent: 1, name: "cache.get", startMs: 10, endMs: 30},
		{id: 3, parent: 1, name: "db.query", startMs: 10, endMs: 90},
	})
	cp := criticalPathOf(t, traces)

	top := dominantSegment(cp)
	assert.Equal(t, "db.query", top.SpanName)
	assert.Equal(t, uint64(80_000), top.SelfTimeUs)
	for _, seg := range cp.Segments {
		assert.NotEqual(t, spanIDOf(2), seg.SpanID, "an off-path parallel branch must not be a critical path segment")
	}
}
