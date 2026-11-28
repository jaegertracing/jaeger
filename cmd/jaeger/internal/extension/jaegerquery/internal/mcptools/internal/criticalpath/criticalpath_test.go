// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// createTestTrace creates a test trace based on test1 from the UI:
//
//	┌──────────────────────────────────────┐
//	│             Span C                   │
//	└──┬──────────▲─────────┬──────────▲───┘
//	+++│          │+++++++++│          │++++
//	   │          │         │          │
//	   ▼──────────┤         ▼──────────┤
//	   │ Span D   │         │ Span E   │
//	   └──────────┘         └──────────┘
//	   +++++++++++          ++++++++++++
//
// Span C: starts at 1μs, duration 100μs (ends at 101μs)
// Span D: starts at 20μs, duration 20μs (ends at 40μs) - child of C
// Span E: starts at 50μs, duration 10μs (ends at 60μs) - child of C
func createTestTrace1() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	// Span C (root)
	spanC := ss.Spans().AppendEmpty()
	spanC.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	spanC.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	spanC.SetStartTimestamp(pcommon.Timestamp(1 * 1000)) // 1μs in nanoseconds
	spanC.SetEndTimestamp(pcommon.Timestamp(101 * 1000)) // 101μs
	spanC.SetName("operation C")

	// Span D (child of C)
	spanD := ss.Spans().AppendEmpty()
	spanD.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	spanD.SetParentSpanID(spanC.SpanID())
	spanD.SetTraceID(spanC.TraceID())
	spanD.SetStartTimestamp(pcommon.Timestamp(20 * 1000)) // 20μs
	spanD.SetEndTimestamp(pcommon.Timestamp(40 * 1000))   // 40μs
	spanD.SetName("operation D")

	// Span E (child of C)
	spanE := ss.Spans().AppendEmpty()
	spanE.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3})
	spanE.SetParentSpanID(spanC.SpanID())
	spanE.SetTraceID(spanC.TraceID())
	spanE.SetStartTimestamp(pcommon.Timestamp(50 * 1000)) // 50μs
	spanE.SetEndTimestamp(pcommon.Timestamp(60 * 1000))   // 60μs
	spanE.SetName("operation E")

	return traces
}

func TestComputeCriticalPath_Test1(t *testing.T) {
	traces := createTestTrace1()

	criticalPath, err := ComputeCriticalPathFromTraces(traces)
	require.NoError(t, err)
	require.NotNil(t, criticalPath)

	// Expected critical path sections from test1
	// The critical path should be:
	// 1. Span C: 60-101μs (after span E finishes)
	// 2. Span E: 50-60μs (span E execution)
	// 3. Span C: 40-50μs (between span D and E)
	// 4. Span D: 20-40μs (span D execution)
	// 5. Span C: 1-20μs (before span D starts)

	expected := []Section{
		{SpanID: "0000000000000001", SectionStart: 60, SectionEnd: 101},
		{SpanID: "0000000000000003", SectionStart: 50, SectionEnd: 60},
		{SpanID: "0000000000000001", SectionStart: 40, SectionEnd: 50},
		{SpanID: "0000000000000002", SectionStart: 20, SectionEnd: 40},
		{SpanID: "0000000000000001", SectionStart: 1, SectionEnd: 20},
	}

	assert.Len(t, criticalPath, len(expected), "Number of critical path sections should match")

	for i, section := range criticalPath {
		assert.Equal(t, expected[i].SpanID, section.SpanID, "Section %d: SpanID should match", i)
		assert.Equal(t, expected[i].SectionStart, section.SectionStart, "Section %d: SectionStart should match", i)
		assert.Equal(t, expected[i].SectionEnd, section.SectionEnd, "Section %d: SectionEnd should match", i)
	}
}

func TestComputeCriticalPath_EmptyTrace(t *testing.T) {
	traces := ptrace.NewTraces()

	criticalPath, err := ComputeCriticalPathFromTraces(traces)
	require.Error(t, err)
	assert.Nil(t, criticalPath)
	assert.Contains(t, err.Error(), "no root span")
}

func TestComputeCriticalPath_NoRootSpan(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	// Create a span with a parent (no root span in trace)
	span := ss.Spans().AppendEmpty()
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	span.SetParentSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 99}) // parent not in trace
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	span.SetStartTimestamp(pcommon.Timestamp(1000))
	span.SetEndTimestamp(pcommon.Timestamp(2000))

	criticalPath, err := ComputeCriticalPathFromTraces(traces)
	require.Error(t, err)
	assert.Nil(t, criticalPath)
	assert.Contains(t, err.Error(), "no root span found")
}

func TestComputeCriticalPath_SingleSpan(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	// Single root span
	span := ss.Spans().AppendEmpty()
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	span.SetStartTimestamp(pcommon.Timestamp(1000)) // 1μs
	span.SetEndTimestamp(pcommon.Timestamp(101000)) // 101μs
	span.SetName("single span")

	criticalPath, err := ComputeCriticalPathFromTraces(traces)
	require.NoError(t, err)
	require.Len(t, criticalPath, 1)

	// The entire span should be on the critical path
	assert.Equal(t, "0000000000000001", criticalPath[0].SpanID)
	assert.Equal(t, uint64(1), criticalPath[0].SectionStart)
	assert.Equal(t, uint64(101), criticalPath[0].SectionEnd)
}

func TestComputeCriticalPath_ZeroDurationRoot(t *testing.T) {
	// Regression test: a valid trace whose critical path is legitimately empty
	// (here a single zero-duration root span) must return an empty result, not an
	// error. Previously this returned "error while computing critical path for
	// trace" because an empty (nil) result was treated as a failure.
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	span.SetStartTimestamp(pcommon.Timestamp(1000))
	span.SetEndTimestamp(pcommon.Timestamp(1000)) // zero duration => empty critical path
	span.SetName("instant span")

	criticalPath, err := ComputeCriticalPathFromTraces(traces)
	require.NoError(t, err)
	require.NotNil(t, criticalPath)
	assert.Empty(t, criticalPath)
}

func TestComputeCriticalPath_Internal_SpanNotFound(t *testing.T) {
	// Test the case where spanID is not in spanMap
	spanMap := map[pcommon.SpanID]CPSpan{
		[8]byte{2}: {SpanID: [8]byte{2}},
	}
	var spanID pcommon.SpanID = [8]byte{1}

	result := computeCriticalPath(spanMap, spanID, nil, nil)
	assert.Nil(t, result)
}

func TestComputeCriticalPath_Internal_LastFinishingChild_Recursive(t *testing.T) {
	// Simple integration test for computeCriticalPath to verify recursion logic
	// Parent -> Child
	spanMap := map[pcommon.SpanID]CPSpan{
		[8]byte{1}: {
			SpanID:       [8]byte{1},
			StartTime:    100,
			Duration:     100,
			ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
		},
		[8]byte{2}: {
			SpanID:       [8]byte{2},
			ParentSpanID: [8]byte{1},
			StartTime:    120,
			Duration:     50,
		},
	}

	result := computeCriticalPath(spanMap, [8]byte{1}, nil, nil)
	require.Len(t, result, 3)
	// 1. span 1: 170-200 (after child ends)
	// 2. span 2: 120-170
	// 3. span 1: 100-120 (before child starts)
}

func TestComputeCriticalPath_ZeroDurationChildAtReturningBoundary(t *testing.T) {
	spanMap := map[pcommon.SpanID]CPSpan{
		[8]byte{1}: {
			SpanID:       [8]byte{1},
			StartTime:    0,
			Duration:     200,
			ChildSpanIDs: []pcommon.SpanID{{2}, {3}},
		},
		[8]byte{2}: {
			SpanID:       [8]byte{2},
			ParentSpanID: [8]byte{1},
			StartTime:    100,
			Duration:     50,
		},
		[8]byte{3}: {
			SpanID:       [8]byte{3},
			ParentSpanID: [8]byte{1},
			StartTime:    100,
			Duration:     0,
		},
	}

	result := computeCriticalPath(spanMap, [8]byte{1}, nil, nil)
	require.Equal(t, []Section{
		{SpanID: "0100000000000000", SectionStart: 150, SectionEnd: 200},
		{SpanID: "0200000000000000", SectionStart: 100, SectionEnd: 150},
		{SpanID: "0100000000000000", SectionStart: 0, SectionEnd: 100},
	}, result)
}

func newSpanID(n uint64) pcommon.SpanID {
	var id pcommon.SpanID
	binary.BigEndian.PutUint64(id[:], n)
	return id
}

func appendSpan(ss ptrace.ScopeSpans, spanID, parentSpanID pcommon.SpanID, startUs, endUs uint64) {
	span := ss.Spans().AppendEmpty()
	span.SetTraceID([16]byte{15: 1})
	span.SetSpanID(spanID)
	span.SetParentSpanID(parentSpanID)
	span.SetStartTimestamp(pcommon.Timestamp(startUs * 1000))
	span.SetEndTimestamp(pcommon.Timestamp(endUs * 1000))
}

func TestComputeCriticalPath_DuplicateSpanIDs(t *testing.T) {
	root, a, b := newSpanID(0x01), newSpanID(0x0a), newSpanID(0x0b)
	var noParent pcommon.SpanID
	type span struct {
		id, parent pcommon.SpanID
		start, end uint64
	}

	tests := []struct {
		name     string
		spans    []span
		expected []Section
		err      string
	}{
		{
			name: "second span with the root's ID is its child",
			spans: []span{
				{root, noParent, 10, 110},
				{root, root, 20, 100},
			},
			expected: []Section{
				{SpanID: root.String(), SectionStart: 10, SectionEnd: 110},
			},
		},
		{
			name: "span reuses the ID of its grandparent",
			spans: []span{
				{root, noParent, 0, 100},
				{a, root, 10, 90},
				{b, a, 20, 80},
				{a, b, 30, 70},
			},
			expected: []Section{
				{SpanID: root.String(), SectionStart: 90, SectionEnd: 100},
				{SpanID: a.String(), SectionStart: 80, SectionEnd: 90},
				{SpanID: b.String(), SectionStart: 20, SectionEnd: 80},
				{SpanID: a.String(), SectionStart: 10, SectionEnd: 20},
				{SpanID: root.String(), SectionStart: 0, SectionEnd: 10},
			},
		},
		{
			name: "root reuses the ID of an earlier span",
			spans: []span{
				{a, b, 10, 90},
				{b, a, 20, 80},
				{a, noParent, 0, 100},
			},
			err: "no root span found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
			for _, s := range tt.spans {
				appendSpan(ss, s.id, s.parent, s.start, s.end)
			}

			criticalPath, err := ComputeCriticalPathFromTraces(traces)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, criticalPath)
		})
	}
}

func TestComputeCriticalPath_DeepChain(t *testing.T) {
	// Each span is the only child of the previous one and ends one microsecond
	// before it, so the walk goes all the way down the chain and back up.
	const depth = 100_000
	traces := ptrace.NewTraces()
	ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	ss.Spans().EnsureCapacity(depth)
	var parent pcommon.SpanID
	for i := range uint64(depth) {
		id := newSpanID(i + 1)
		appendSpan(ss, id, parent, i, 2*depth-i)
		parent = id
	}

	criticalPath, err := ComputeCriticalPathFromTraces(traces)
	require.NoError(t, err)
	require.Len(t, criticalPath, 2*depth-1)
	var total uint64
	for _, section := range criticalPath {
		total += section.SectionEnd - section.SectionStart
	}
	assert.Equal(t, uint64(2*depth), total)
}

func TestFindLastFinishingChildSpan_MissingChild(t *testing.T) {
	// Test findLastFinishingChildSpan with child ID in list but missing from map (find_lfc.go line 23)
	parentSpan := CPSpan{
		SpanID:       [8]byte{1},
		ChildSpanIDs: []pcommon.SpanID{[8]byte{2}}, // Refers to missing span
	}
	spanMap := map[pcommon.SpanID]CPSpan{} // Empty map

	result := findLastFinishingChildSpan(spanMap, parentSpan, nil)
	assert.Nil(t, result)
}

// TestComputeCriticalPath_GrandchildCascadeViaPublicAPI is a regression test
// for the clamping-order bug exercised through the public
// ComputeCriticalPathFromTraces entry point.
//
// Scenario (timestamps in microseconds, stored as nanoseconds in ptrace):
//
//	A [0..100]  root
//	  └─ B [50..150]  overflows A, clamped to [50..100]
//	       └─ C [120..140]  within B's original range but outside B's clamped range
//
// With the old single-pass code, map iteration order determined whether B was
// clamped before C was checked. If C was visited first, B still had its
// original end (150), so C's start (120) fell inside [50..150] and C passed.
// B was then clamped to [50..100], leaving C (start 120) stale and outside its
// parent. The BFS guarantees B is resolved before C is evaluated, so C is
// always dropped because 120 >= 100 (B's clamped end).
//
// Expected outcome: B survives, clamped to [50..100]. C is dropped.
// C must not appear in the critical path output.
//
// The test is repeated 100 times because Go randomises map iteration; a single
// failure proves the bug.
func TestComputeCriticalPath_GrandchildCascadeViaPublicAPI(t *testing.T) {
	spA := pcommon.SpanID([8]byte{0xA})
	spB := pcommon.SpanID([8]byte{0xB})
	spC := pcommon.SpanID([8]byte{0xC})

	buildTrace := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

		a := ss.Spans().AppendEmpty()
		a.SetSpanID(spA)
		a.SetTraceID([16]byte{1})
		a.SetStartTimestamp(pcommon.Timestamp(0 * 1000))
		a.SetEndTimestamp(pcommon.Timestamp(100 * 1000)) // A: [0..100]

		b := ss.Spans().AppendEmpty()
		b.SetSpanID(spB)
		b.SetParentSpanID(spA)
		b.SetTraceID([16]byte{1})
		b.SetStartTimestamp(pcommon.Timestamp(50 * 1000)) // B: [50..150], overlaps A
		b.SetEndTimestamp(pcommon.Timestamp(150 * 1000))  // clamped to [50..100]

		c := ss.Spans().AppendEmpty()
		c.SetSpanID(spC)
		c.SetParentSpanID(spB)
		c.SetTraceID([16]byte{1})
		c.SetStartTimestamp(pcommon.Timestamp(120 * 1000)) // C: [120..140]
		c.SetEndTimestamp(pcommon.Timestamp(140 * 1000))   // inside B's original range, outside B's clamped range

		return traces
	}

	const iterations = 100
	for i := range iterations {
		sections, err := ComputeCriticalPathFromTraces(buildTrace())
		require.NoError(t, err, "iteration %d", i)

		// B survives (clamped); C is dropped because its start (120) is
		// outside B's clamped range [50..100]. C must not appear in the output.
		for _, sec := range sections {
			assert.NotEqual(t, spC.String(), sec.SpanID,
				"iteration %d: C is outside B's clamped range and must not appear in critical path", i)
		}

		// A must appear in the critical path.
		found := false
		for _, sec := range sections {
			if sec.SpanID == spA.String() {
				found = true
				break
			}
		}
		assert.True(t, found, "iteration %d: root A must appear in critical path", i)

		// B must appear in the critical path (it survived, clamped to [50..100]).
		foundB := false
		for _, sec := range sections {
			if sec.SpanID == spB.String() {
				foundB = true
				// B was clamped to [50..100]; no section for B may fall outside that range.
				assert.GreaterOrEqual(t, sec.SectionStart, uint64(50),
					"iteration %d: B section start must be >= 50", i)
				assert.LessOrEqual(t, sec.SectionEnd, uint64(100),
					"iteration %d: B section end must be <= 100 (B's clamped end)", i)
			}
		}
		assert.True(t, foundB, "iteration %d: clamped B must appear in critical path", i)
	}
}
