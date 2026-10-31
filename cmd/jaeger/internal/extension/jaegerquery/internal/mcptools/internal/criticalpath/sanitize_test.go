// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestSanitizeOverFlowingChildren(t *testing.T) {
	tests := []struct {
		name     string
		input    map[pcommon.SpanID]CPSpan
		expected map[pcommon.SpanID]CPSpan
	}{
		{
			name: "child within parent bounds",
			input: map[pcommon.SpanID]CPSpan{
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
			},
			expected: map[pcommon.SpanID]CPSpan{
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
			},
		},
		{
			name: "child starts after parent ends - should be dropped",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    250,
					Duration:     50,
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{}, // child removed
				},
			},
		},
		{
			name: "child ends after parent - should be truncated",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    150,
					Duration:     100, // ends at 250, parent ends at 200
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    150,
					Duration:     50, // truncated to fit parent
				},
			},
		},
		{
			name: "child ends before parent starts - should be dropped",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    50,
					Duration:     40, // ends at 90
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{}, // child removed
				},
			},
		},
		{
			name: "child starts before parent - should be truncated",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    50,
					Duration:     100, // ends at 150
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    100, // adjusted to parent start
					Duration:     50,  // adjusted duration
				},
			},
		},
		{
			name: "child starts before and ends after parent - should be truncated",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    50,
					Duration:     200, // starts at 50, ends at 250
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					StartTime:    100,
					Duration:     100,
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1},
					StartTime:    100, // adjusted to parent start
					Duration:     100, // adjusted to parent duration
				},
			},
		},
		{
			name: "child with dropped parent - should be dropped",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ParentSpanID: [8]byte{1}, // parent doesn't exist
					StartTime:    100,
					Duration:     50,
				},
			},
			expected: map[pcommon.SpanID]CPSpan{}, // child should be dropped
		},
		{
			name: "root span without parent",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:    [8]byte{1},
					StartTime: 100,
					Duration:  100,
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:    [8]byte{1},
					StartTime: 100,
					Duration:  100,
				},
			},
		},
		{
			name:     "empty span map",
			input:    map[pcommon.SpanID]CPSpan{},
			expected: map[pcommon.SpanID]CPSpan{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data
			inputCopy := make(map[pcommon.SpanID]CPSpan)
			maps.Copy(inputCopy, tt.input)

			result := removeOverflowingChildren(inputCopy)
			assert.Len(t, result, len(tt.expected), "unexpected number of spans")

			for spanID, expectedSpan := range tt.expected {
				actualSpan, ok := result[spanID]
				assert.True(t, ok, "expected span %v not found in result", spanID)
				if !ok {
					continue
				}

				assert.Equal(t, expectedSpan.StartTime, actualSpan.StartTime,
					"span %v: start time mismatch", spanID)
				assert.Equal(t, expectedSpan.Duration, actualSpan.Duration,
					"span %v: duration mismatch", spanID)
				assert.Len(t, actualSpan.ChildSpanIDs, len(expectedSpan.ChildSpanIDs),
					"span %v: child count mismatch", spanID)
			}
		})
	}
}

func TestSanitizeOverFlowingChildren_MultipleChildren(t *testing.T) {
	// Test loops that filter children when one is removed and others stay.
	input := map[pcommon.SpanID]CPSpan{
		[8]byte{1}: {
			SpanID:       [8]byte{1},
			StartTime:    100,
			Duration:     100, // 100-200
			ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}, [8]byte{4}},
		},
		[8]byte{2}: { // Valid child
			SpanID:       [8]byte{2},
			ParentSpanID: [8]byte{1},
			StartTime:    120,
			Duration:     50, // 120-170
		},
		[8]byte{3}: { // Invalid: starts after parent ends
			SpanID:       [8]byte{3},
			ParentSpanID: [8]byte{1},
			StartTime:    250,
			Duration:     50,
		},
		[8]byte{4}: { // Invalid: ends before parent starts
			SpanID:       [8]byte{4},
			ParentSpanID: [8]byte{1},
			StartTime:    50,
			Duration:     20, // 50-70
		},
	}

	result := removeOverflowingChildren(input)

	// Span 1 should have only Span 2 as child
	parent := result[[8]byte{1}]
	assert.Len(t, parent.ChildSpanIDs, 1)
	assert.Equal(t, pcommon.SpanID([8]byte{2}), parent.ChildSpanIDs[0])

	// Span 3 and 4 should be removed
	_, ok3 := result[[8]byte{3}]
	assert.False(t, ok3)
	_, ok4 := result[[8]byte{4}]
	assert.False(t, ok4)
}

// TestSanitizeOverFlowingChildren_OrphanCascade pins the behavior of a "true
// orphan": a span whose ParentSpanID is non-empty but does not exist anywhere
// in spanMap (its parent was never part of the trace, not merely dropped during
// this pass).
//
// The original single-pass code dropped the orphan via the `parentExists ==
// false` branch (delete + continue, no explicit child cascade). Children of the
// orphan were still in the flat spanIDs snapshot, so they were visited later;
// their own parent-lookup also failed (orphan was deleted), causing them to be
// deleted too. The net result: the full subtree rooted at the orphan is removed.
//
// The BFS rewrite replicates this by enqueuing the orphan's children
// (appendChildren) before the continue, so they are processed in the same pass
// and deleted via the same orphan-detection branch. The observable result is
// identical: every span in the orphan's subtree is absent from the output.
//
// This test explicitly asserts that contract for a two-level subtree:
//
//	Orphan O  (ParentSpanID → X, X not in map)
//	  └─ Child C
//	       └─ Grandchild G
func TestSanitizeOverFlowingChildren_OrphanCascade(t *testing.T) {
	spO := pcommon.SpanID([8]byte{0xE1}) // orphan: parent missing from map
	spC := pcommon.SpanID([8]byte{0xE2}) // child of orphan
	spG := pcommon.SpanID([8]byte{0xE3}) // grandchild of orphan
	spX := pcommon.SpanID([8]byte{0xFF}) // the missing parent (never in map)

	input := map[pcommon.SpanID]CPSpan{
		spO: {
			SpanID:       spO,
			ParentSpanID: spX, // points outside the map
			StartTime:    100,
			Duration:     200,
			ChildSpanIDs: []pcommon.SpanID{spC},
		},
		spC: {
			SpanID:       spC,
			ParentSpanID: spO,
			StartTime:    110,
			Duration:     50,
			ChildSpanIDs: []pcommon.SpanID{spG},
		},
		spG: {
			SpanID:       spG,
			ParentSpanID: spC,
			StartTime:    120,
			Duration:     20,
		},
	}

	result := removeOverflowingChildren(input)

	// The entire subtree must be absent.
	_, okO := result[spO]
	assert.False(t, okO, "orphan O must be dropped (its parent is not in the map)")

	_, okC := result[spC]
	assert.False(t, okC, "child C of orphan O must be cascade-dropped")

	_, okG := result[spG]
	assert.False(t, okG, "grandchild G of orphan O must be cascade-dropped")

	assert.Empty(t, result, "result must be completely empty")
}

// TestSanitizeOverFlowingChildren_GrandchildCascade is a regression test for
// a non-deterministic bug where a grandchild could survive sanitization if its
// intermediate parent (which overflows the grandparent) happened to be iterated
// after the grandchild in the old random-order map pass.
//
// Scenario:
//
//	Root A [0..200]
//	  └─ B [210..300]   overflows A → must be dropped
//	       └─ C [220..280]  grandchild via B → must also be dropped
//
// With the old implementation, if C was visited before B, C's parent (B) was
// still present in the map at that moment, so C survived the single pass. B was
// then deleted, leaving C as an orphan. The test is run many times to detect
// non-determinism: even a single failure proves the bug.
func TestSanitizeOverFlowingChildren_GrandchildCascade(t *testing.T) {
	// spA, spB, spC use distinct byte values so the map has three different keys.
	spA := pcommon.SpanID([8]byte{0xA})
	spB := pcommon.SpanID([8]byte{0xB})
	spC := pcommon.SpanID([8]byte{0xC})

	buildInput := func() map[pcommon.SpanID]CPSpan {
		return map[pcommon.SpanID]CPSpan{
			spA: {
				SpanID:       spA,
				StartTime:    0,
				Duration:     200, // [0..200]
				ChildSpanIDs: []pcommon.SpanID{spB},
			},
			spB: {
				SpanID:       spB,
				ParentSpanID: spA,
				StartTime:    210, // starts after A ends → overflows
				Duration:     90,  // [210..300]
				ChildSpanIDs: []pcommon.SpanID{spC},
			},
			spC: {
				SpanID:       spC,
				ParentSpanID: spB,
				StartTime:    220, // grandchild of A via overflowing B
				Duration:     60,  // [220..280]
			},
		}
	}

	// Run many times: Go map iteration is randomized per run, so a bug in the
	// old single-pass implementation would surface on a fraction of iterations.
	// 100 runs is enough to make a flaky pass astronomically unlikely.
	const iterations = 100
	for i := range iterations {
		result := removeOverflowingChildren(buildInput())

		// A must survive with an empty ChildSpanIDs list (B was removed from it).
		a, ok := result[spA]
		assert.True(t, ok, "iteration %d: root A must survive", i)
		assert.Empty(t, a.ChildSpanIDs,
			"iteration %d: A's ChildSpanIDs must be empty after B is dropped", i)

		// B overflows A and must be dropped.
		_, ok = result[spB]
		assert.False(t, ok, "iteration %d: overflowing B must be dropped", i)

		// C is a descendant of dropped B and must also be dropped.
		_, ok = result[spC]
		assert.False(t, ok, "iteration %d: grandchild C must be dropped with B", i)

		// No span in the result may reference B or C in its ChildSpanIDs.
		for id, span := range result {
			for _, childID := range span.ChildSpanIDs {
				assert.NotEqual(t, spB, childID,
					"iteration %d: span %v still references dropped B", i, id)
				assert.NotEqual(t, spC, childID,
					"iteration %d: span %v still references dropped C", i, id)
			}
		}
	}
}

// TestSanitizeOverFlowingChildren_CascadeDanglingChild ensures that if a dropped span
// references a child that does not exist in the span map, cascadeDelete safely ignores it.
func TestSanitizeOverFlowingChildren_CascadeDanglingChild(t *testing.T) {
	spA := pcommon.SpanID([8]byte{0xA})
	spB := pcommon.SpanID([8]byte{0xB})
	spMissing := pcommon.SpanID([8]byte{0xEE})

	input := map[pcommon.SpanID]CPSpan{
		spA: {
			SpanID:       spA,
			StartTime:    0,
			Duration:     200,
			ChildSpanIDs: []pcommon.SpanID{spB},
		},
		spB: {
			SpanID:       spB,
			ParentSpanID: spA,
			StartTime:    250, // overflows A, will be dropped
			Duration:     50,
			ChildSpanIDs: []pcommon.SpanID{spMissing}, // references missing child
		},
	}

	result := removeOverflowingChildren(input)

	// A survives with empty children
	a, ok := result[spA]
	assert.True(t, ok)
	assert.Empty(t, a.ChildSpanIDs)

	// B is dropped
	_, ok = result[spB]
	assert.False(t, ok)
}

// TestSanitizeOverFlowingChildren_CycleDetection ensures that spans forming a cycle
// (e.g. A -> B -> A) do not cause an infinite loop and that cyclic back-edges are broken.
func TestSanitizeOverFlowingChildren_CycleDetection(t *testing.T) {
	spRoot := pcommon.SpanID([8]byte{1})
	spA := pcommon.SpanID([8]byte{2})
	spB := pcommon.SpanID([8]byte{3})

	input := map[pcommon.SpanID]CPSpan{
		spRoot: {
			SpanID:       spRoot,
			StartTime:    0,
			Duration:     1000,
			ChildSpanIDs: []pcommon.SpanID{spA},
		},
		spA: {
			SpanID:       spA,
			ParentSpanID: spRoot,
			StartTime:    100,
			Duration:     400,
			ChildSpanIDs: []pcommon.SpanID{spB},
		},
		spB: {
			SpanID:       spB,
			ParentSpanID: spA,
			StartTime:    150,
			Duration:     200,
			ChildSpanIDs: []pcommon.SpanID{spA}, // cycle back to A!
		},
	}

	result := removeOverflowingChildren(input)

	// All spans should survive, but the cycle must be broken (spB must not reference spA as child).
	b, okB := result[spB]
	assert.True(t, okB)
	assert.NotContains(t, b.ChildSpanIDs, spA)
}

// TestSanitizeOverFlowingChildren_DisconnectedCycle ensures that disconnected cyclic spans
// with no root span are dropped rather than surviving un-sanitized.
func TestSanitizeOverFlowingChildren_DisconnectedCycle(t *testing.T) {
	spA := pcommon.SpanID([8]byte{0xAA})
	spB := pcommon.SpanID([8]byte{0xBB})

	input := map[pcommon.SpanID]CPSpan{
		spA: {
			SpanID:       spA,
			ParentSpanID: spB,
			StartTime:    100,
			Duration:     100,
			ChildSpanIDs: []pcommon.SpanID{spB},
		},
		spB: {
			SpanID:       spB,
			ParentSpanID: spA,
			StartTime:    100,
			Duration:     100,
			ChildSpanIDs: []pcommon.SpanID{spA},
		},
	}

	result := removeOverflowingChildren(input)
	assert.Empty(t, result)
}

// TestSanitizeOverFlowingChildren_SelfReferenceStandalone ensures that an isolated span
// that references itself as its parent (ParentSpanID == SpanID) is purged as an invalid cycle.
func TestSanitizeOverFlowingChildren_SelfReferenceStandalone(t *testing.T) {
	spA := pcommon.SpanID([8]byte{0x55})

	input := map[pcommon.SpanID]CPSpan{
		spA: {
			SpanID:       spA,
			ParentSpanID: spA, // points to itself
			StartTime:    100,
			Duration:     100,
			ChildSpanIDs: []pcommon.SpanID{spA},
		},
	}

	result := removeOverflowingChildren(input)
	assert.Empty(t, result)
}

// TestSanitizeOverFlowingChildren_SelfReferenceChild ensures that a valid child span
// listing itself in its ChildSpanIDs has the self-referencing loop pruned.
func TestSanitizeOverFlowingChildren_SelfReferenceChild(t *testing.T) {
	spRoot := pcommon.SpanID([8]byte{1})
	spA := pcommon.SpanID([8]byte{2})

	input := map[pcommon.SpanID]CPSpan{
		spRoot: {
			SpanID:       spRoot,
			StartTime:    0,
			Duration:     1000,
			ChildSpanIDs: []pcommon.SpanID{spA},
		},
		spA: {
			SpanID:       spA,
			ParentSpanID: spRoot,
			StartTime:    100,
			Duration:     200,
			ChildSpanIDs: []pcommon.SpanID{spA}, // self-loop
		},
	}

	result := removeOverflowingChildren(input)
	a, ok := result[spA]
	assert.True(t, ok)
	assert.NotContains(t, a.ChildSpanIDs, spA)
}

// TestSanitizeOverFlowingChildren_DeterministicOrder runs cycle sanitization across multiple
// iterations to ensure the result is completely deterministic despite Go's randomized map iteration.
func TestSanitizeOverFlowingChildren_DeterministicOrder(t *testing.T) {
	spRoot1 := pcommon.SpanID([8]byte{1})
	spRoot2 := pcommon.SpanID([8]byte{2})
	spA := pcommon.SpanID([8]byte{3})
	spB := pcommon.SpanID([8]byte{4})

	buildInput := func() map[pcommon.SpanID]CPSpan {
		return map[pcommon.SpanID]CPSpan{
			spRoot1: {
				SpanID:       spRoot1,
				StartTime:    0,
				Duration:     500,
				ChildSpanIDs: []pcommon.SpanID{spA},
			},
			spRoot2: {
				SpanID:       spRoot2,
				StartTime:    50,
				Duration:     500,
				ChildSpanIDs: []pcommon.SpanID{spB},
			},
			spA: {
				SpanID:       spA,
				ParentSpanID: spRoot1,
				StartTime:    100,
				Duration:     200,
				ChildSpanIDs: []pcommon.SpanID{spB},
			},
			spB: {
				SpanID:       spB,
				ParentSpanID: spRoot2,
				StartTime:    150,
				Duration:     200,
				ChildSpanIDs: []pcommon.SpanID{spA}, // creates cycle A <-> B
			},
		}
	}

	for i := range 50 {
		res := removeOverflowingChildren(buildInput())
		assert.Len(t, res, 4, "iteration %d: all spans must survive with cycle broken", i)
		// spA was visited first because spRoot1 has StartTime 0 < spRoot2 StartTime 50.
		// Thus the back-edge spB -> spA must be pruned.
		b := res[spB]
		assert.NotContains(t, b.ChildSpanIDs, spA, "iteration %d: spB must not contain spA", i)
	}
}

// TestSanitizeOverFlowingChildren_DeterministicOrder_EqualStartTime exercises the tie-breaker
// comparison when two seed spans share the exact same StartTime.
func TestSanitizeOverFlowingChildren_DeterministicOrder_EqualStartTime(t *testing.T) {
	spRoot1 := pcommon.SpanID([8]byte{0x01})
	spRoot2 := pcommon.SpanID([8]byte{0x02})

	input := map[pcommon.SpanID]CPSpan{
		spRoot1: {
			SpanID:    spRoot1,
			StartTime: 100, // same start time
			Duration:  200,
		},
		spRoot2: {
			SpanID:    spRoot2,
			StartTime: 100, // same start time
			Duration:  200,
		},
	}

	res := removeOverflowingChildren(input)
	assert.Len(t, res, 2)
}
