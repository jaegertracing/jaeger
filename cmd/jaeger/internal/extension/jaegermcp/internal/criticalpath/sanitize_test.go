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

func TestSanitizeOverFlowingChildren_ThreeLevel(t *testing.T) {
	// Test three-level trace to ensure parent removal cascades deterministically to descendants.
	// A (root, 100-200) -> B (child, 120-150) -> C (grandchild, 130-140)
	input := map[pcommon.SpanID]CPSpan{
		[8]byte{1}: {
			SpanID:       [8]byte{1},
			StartTime:    100,
			Duration:     100, // 100-200
			ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
		},
		[8]byte{2}: {
			SpanID:       [8]byte{2},
			ParentSpanID: [8]byte{1},
			StartTime:    120,
			Duration:     30, // 120-150
			ChildSpanIDs: []pcommon.SpanID{[8]byte{3}},
		},
		[8]byte{3}: {
			SpanID:       [8]byte{3},
			ParentSpanID: [8]byte{2},
			StartTime:    130,
			Duration:     10, // 130-140
		},
	}

	result := removeOverflowingChildren(input)

	// All three spans should be present and valid
	assert.Len(t, result, 3)
	assert.Equal(t, uint64(100), result[[8]byte{1}].StartTime)
	assert.Equal(t, uint64(120), result[[8]byte{2}].StartTime)
	assert.Equal(t, uint64(130), result[[8]byte{3}].StartTime)
}

func TestSanitizeOverFlowingChildren_CascadingDrop(t *testing.T) {
	// Test that when a parent is dropped, all descendants are also dropped deterministically.
	// A (root, 100-200) -> B (child, 250-300, overflows parent) -> C (grandchild, 260-280)
	input := map[pcommon.SpanID]CPSpan{
		[8]byte{1}: {
			SpanID:       [8]byte{1},
			StartTime:    100,
			Duration:     100, // 100-200
			ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
		},
		[8]byte{2}: {
			SpanID:       [8]byte{2},
			ParentSpanID: [8]byte{1},
			StartTime:    250, // Completely overflows parent
			Duration:     50,  // 250-300
			ChildSpanIDs: []pcommon.SpanID{[8]byte{3}},
		},
		[8]byte{3}: {
			SpanID:       [8]byte{3},
			ParentSpanID: [8]byte{2},
			StartTime:    260,
			Duration:     20, // 260-280
		},
	}

	result := removeOverflowingChildren(input)

	// Only span 1 should remain; spans 2 and 3 should be dropped
	assert.Len(t, result, 1)
	assert.NotNil(t, result[[8]byte{1}])
	assert.Empty(t, result[[8]byte{1}].ChildSpanIDs) // Child removed from parent

	// Verify spans 2 and 3 are gone
	_, ok2 := result[[8]byte{2}]
	assert.False(t, ok2)
	_, ok3 := result[[8]byte{3}]
	assert.False(t, ok3)
}

func TestSanitizeOverFlowingChildren_DeterministicOrder(t *testing.T) {
	// Test that multiple calls with the same input produce consistent output.
	// This test verifies that map iteration randomness doesn't affect results.
	// We run the function multiple times and ensure the output is identical each time.

	createTestInput := func() map[pcommon.SpanID]CPSpan {
		return map[pcommon.SpanID]CPSpan{
			[8]byte{1}: {
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}, [8]byte{4}, [8]byte{5}},
			},
			[8]byte{2}: {
				SpanID:       [8]byte{2},
				ParentSpanID: [8]byte{1},
				StartTime:    120,
				Duration:     50,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{6}},
			},
			[8]byte{3}: { // Overflows parent, should be dropped
				SpanID:       [8]byte{3},
				ParentSpanID: [8]byte{1},
				StartTime:    250,
				Duration:     50,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{7}},
			},
			[8]byte{4}: { // Valid
				SpanID:       [8]byte{4},
				ParentSpanID: [8]byte{1},
				StartTime:    110,
				Duration:     30,
			},
			[8]byte{5}: { // Before parent, should be dropped
				SpanID:       [8]byte{5},
				ParentSpanID: [8]byte{1},
				StartTime:    50,
				Duration:     20,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{8}},
			},
			[8]byte{6}: { // Grandchild of 2
				SpanID:       [8]byte{6},
				ParentSpanID: [8]byte{2},
				StartTime:    130,
				Duration:     10,
			},
			[8]byte{7}: { // Grandchild of 3 (dropped parent), should also be dropped
				SpanID:       [8]byte{7},
				ParentSpanID: [8]byte{3},
				StartTime:    260,
				Duration:     10,
			},
			[8]byte{8}: { // Grandchild of 5 (dropped parent), should also be dropped
				SpanID:       [8]byte{8},
				ParentSpanID: [8]byte{5},
				StartTime:    60,
				Duration:     5,
			},
		}
	}

	// Run the function multiple times and collect results
	const runs = 5
	results := make([]map[pcommon.SpanID]CPSpan, runs)
	for i := 0; i < runs; i++ {
		input := createTestInput()
		results[i] = removeOverflowingChildren(input)
	}

	// Verify all runs produce identical results
	for i := 1; i < runs; i++ {
		assert.Len(t, results[i], len(results[0]), "run %d: different number of spans", i)

		// Check each span matches
		for spanID, expectedSpan := range results[0] {
			actualSpan, ok := results[i][spanID]
			assert.True(t, ok, "run %d: span %v missing", i, spanID)
			if ok {
				assert.Equal(t, expectedSpan.StartTime, actualSpan.StartTime,
					"run %d: span %v start mismatch", i, spanID)
				assert.Equal(t, expectedSpan.Duration, actualSpan.Duration,
					"run %d: span %v duration mismatch", i, spanID)
				assert.Len(t, actualSpan.ChildSpanIDs, len(expectedSpan.ChildSpanIDs),
					"run %d: span %v child count mismatch", i, spanID)

				// Check child IDs match exactly (order matters for determinism)
				for j, childID := range expectedSpan.ChildSpanIDs {
					if j < len(actualSpan.ChildSpanIDs) {
						assert.Equal(t, childID, actualSpan.ChildSpanIDs[j],
							"run %d: span %v child[%d] mismatch", i, spanID, j)
					}
				}
			}
		}
	}

	// Verify the expected spans were kept/dropped
	result := results[0]
	expectedKept := map[pcommon.SpanID]bool{
		[8]byte{1}: true,
		[8]byte{2}: true,
		[8]byte{4}: true,
		[8]byte{6}: true,
	}
	expectedDropped := map[pcommon.SpanID]bool{
		[8]byte{3}: true,
		[8]byte{5}: true,
		[8]byte{7}: true,
		[8]byte{8}: true,
	}

	// Verify kept spans
	for spanID := range expectedKept {
		_, ok := result[spanID]
		assert.True(t, ok, "span %v should have been kept", spanID)
	}

	// Verify dropped spans
	for spanID := range expectedDropped {
		_, ok := result[spanID]
		assert.False(t, ok, "span %v should have been dropped", spanID)
	}
}

func TestSanitizeOverFlowingChildren_CascadeStableAcrossRuns(t *testing.T) {
	// This regression test targets historical nondeterminism caused by map iteration
	// and in-place deletion. Span 3 overflows root and must be dropped with ALL descendants.
	createInput := func() map[pcommon.SpanID]CPSpan {
		return map[pcommon.SpanID]CPSpan{
			[8]byte{1}: {
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100, // 100-200
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}},
			},
			[8]byte{2}: { // valid child
				SpanID:       [8]byte{2},
				ParentSpanID: [8]byte{1},
				StartTime:    120,
				Duration:     30,
			},
			[8]byte{3}: { // invalid child: entirely after parent, must be dropped
				SpanID:       [8]byte{3},
				ParentSpanID: [8]byte{1},
				StartTime:    250,
				Duration:     30,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{4}, [8]byte{5}},
			},
			[8]byte{4}: {
				SpanID:       [8]byte{4},
				ParentSpanID: [8]byte{3},
				StartTime:    255,
				Duration:     10,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{6}},
			},
			[8]byte{5}: {
				SpanID:       [8]byte{5},
				ParentSpanID: [8]byte{3},
				StartTime:    260,
				Duration:     10,
			},
			[8]byte{6}: {
				SpanID:       [8]byte{6},
				ParentSpanID: [8]byte{4},
				StartTime:    257,
				Duration:     5,
			},
		}
	}

	const runs = 200
	for i := 0; i < runs; i++ {
		result := removeOverflowingChildren(createInput())

		// Only root and valid child should remain.
		assert.Len(t, result, 2, "run %d: unexpected result size", i)
		_, ok1 := result[[8]byte{1}]
		_, ok2 := result[[8]byte{2}]
		assert.True(t, ok1, "run %d: root unexpectedly missing", i)
		assert.True(t, ok2, "run %d: valid child unexpectedly missing", i)

		// Entire dropped subtree must never survive.
		_, ok3 := result[[8]byte{3}]
		_, ok4 := result[[8]byte{4}]
		_, ok5 := result[[8]byte{5}]
		_, ok6 := result[[8]byte{6}]
		assert.False(t, ok3, "run %d: dropped span 3 survived", i)
		assert.False(t, ok4, "run %d: dropped descendant span 4 survived", i)
		assert.False(t, ok5, "run %d: dropped descendant span 5 survived", i)
		assert.False(t, ok6, "run %d: dropped descendant span 6 survived", i)
	}
}
