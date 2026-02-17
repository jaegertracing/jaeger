// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
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

func TestComputeCriticalPath_Internal_SpanNotFound(t *testing.T) {
	// Test the case where spanID is not in spanMap (line 51)
	spanMap := map[pcommon.SpanID]CPSpan{}
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
			SpanID:    [8]byte{2},
			StartTime: 120,
			Duration:  50,
			References: []CPSpanReference{
				{RefType: "CHILD_OF", SpanID: [8]byte{1}},
			},
		},
	}

	result := computeCriticalPath(spanMap, [8]byte{1}, nil, nil)
	require.Len(t, result, 3)
	// 1. span 1: 170-200 (after child ends)
	// 2. span 2: 120-170
	// 3. span 1: 100-120 (before child starts)
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
