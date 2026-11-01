// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// runWithTimeout runs fn in a goroutine and fails the test if it does not
// finish within the given timeout. It protects the test runner from hanging
// forever if the recursion guard regresses.
func runWithTimeout(t *testing.T, name string, timeout time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s did not terminate within %v (infinite recursion?)", name, timeout)
	}
}

func TestComputeCriticalPath_CyclicSpanMap(t *testing.T) {
	// A <-> B cycle: A lists B as its child and B lists A as its child.
	// Without cycle detection this recurses indefinitely (issue #9966).
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
			ChildSpanIDs: []pcommon.SpanID{[8]byte{1}},
		},
	}

	var result []Section
	runWithTimeout(t, "computeCriticalPath (cyclic)", 5*time.Second, func() {
		result = computeCriticalPath(spanMap, [8]byte{1}, []Section{}, nil)
	})
	// The recursion guard stops at the repeated visit; a non-nil (possibly
	// partial) result is acceptable as long as it terminates.
	assert.NotNil(t, result)
}

func TestComputeCriticalPath_SelfReferencingSpanMap(t *testing.T) {
	// Self-referencing span: ParentSpanID == SpanID.
	spanMap := map[pcommon.SpanID]CPSpan{
		[8]byte{1}: {
			SpanID:       [8]byte{1},
			ParentSpanID: [8]byte{1}, // self-reference
			StartTime:    100,
			Duration:     100,
		},
	}

	var result []Section
	runWithTimeout(t, "computeCriticalPath (self-referencing)", 5*time.Second, func() {
		result = computeCriticalPath(spanMap, [8]byte{1}, []Section{}, nil)
	})
	assert.NotNil(t, result)
}

func TestComputeCriticalPath_DuplicateSpanID_Trace(t *testing.T) {
	// End-to-end: a trace where a span ID appears twice with different parents
	// (exporter-corrupted). Previously this overwrote spanMap[A] while
	// childrenMap kept both parents, creating a synthetic cycle
	// Root -> A -> B -> A that hung the computation.
	traces := ptrace.NewTraces()
	ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

	// Span A (root).
	spanA1 := ss.Spans().AppendEmpty()
	spanA1.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	spanA1.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	spanA1.SetStartTimestamp(pcommon.Timestamp(1 * 1000))
	spanA1.SetEndTimestamp(pcommon.Timestamp(101 * 1000))
	spanA1.SetName("operation A")

	// Span B, child of A.
	spanB := ss.Spans().AppendEmpty()
	spanB.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	spanB.SetParentSpanID(spanA1.SpanID())
	spanB.SetTraceID(spanA1.TraceID())
	spanB.SetStartTimestamp(pcommon.Timestamp(20 * 1000))
	spanB.SetEndTimestamp(pcommon.Timestamp(40 * 1000))
	spanB.SetName("operation B")

	// Duplicate span ID 1, now as a child of B => creates Root -> A -> B -> A.
	spanA2 := ss.Spans().AppendEmpty()
	spanA2.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	spanA2.SetParentSpanID(spanB.SpanID())
	spanA2.SetTraceID(spanA1.TraceID())
	spanA2.SetStartTimestamp(pcommon.Timestamp(50 * 1000))
	spanA2.SetEndTimestamp(pcommon.Timestamp(60 * 1000))
	spanA2.SetName("operation A (duplicate)")

	var result []Section
	var err error
	runWithTimeout(t, "ComputeCriticalPathFromTraces (duplicate span ID)", 5*time.Second, func() {
		result, err = ComputeCriticalPathFromTraces(traces)
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	// The root span's critical path must still be produced.
	assert.NotEmpty(t, result)
}

func TestCreateCPSpanMap_DuplicateSpanIDs(t *testing.T) {
	// Two spans with the same span ID but different parents: the first
	// occurrence must be kept so that spanMap and childrenMap stay in sync.
	traces := ptrace.NewTraces()
	ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

	spanA1 := ss.Spans().AppendEmpty()
	spanA1.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 9})
	spanA1.SetStartTimestamp(pcommon.Timestamp(1 * 1000))
	spanA1.SetEndTimestamp(pcommon.Timestamp(101 * 1000))

	spanB := ss.Spans().AppendEmpty()
	spanB.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	spanB.SetParentSpanID(spanA1.SpanID())
	spanB.SetStartTimestamp(pcommon.Timestamp(20 * 1000))
	spanB.SetEndTimestamp(pcommon.Timestamp(40 * 1000))

	// Duplicate span ID 9 with a different parent (span C).
	spanC := ss.Spans().AppendEmpty()
	spanC.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3})
	spanC.SetStartTimestamp(pcommon.Timestamp(50 * 1000))
	spanC.SetEndTimestamp(pcommon.Timestamp(60 * 1000))

	spanA2 := ss.Spans().AppendEmpty()
	spanA2.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 9}) // same ID as spanA1, different parent
	spanA2.SetParentSpanID(spanC.SpanID())
	spanA2.SetStartTimestamp(pcommon.Timestamp(10 * 1000))
	spanA2.SetEndTimestamp(pcommon.Timestamp(30 * 1000))

	spanMap := CreateCPSpanMap(traces)
	require.NotNil(t, spanMap)
	// The first occurrence is kept, so the map has 3 distinct IDs.
	assert.Len(t, spanMap, 3)
	// The kept span must be the root (empty parent), not the duplicate child.
	assert.True(t, spanMap[[8]byte{9}].ParentSpanID.IsEmpty())
}
