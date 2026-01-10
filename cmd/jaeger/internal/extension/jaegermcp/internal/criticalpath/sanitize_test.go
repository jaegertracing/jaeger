// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
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
					SpanID:    [8]byte{2},
					StartTime: 120,
					Duration:  50,
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 120,
					Duration:  50,
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 250,
					Duration:  50,
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 150,
					Duration:  100, // ends at 250, parent ends at 200
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 150,
					Duration:  50, // truncated to fit parent
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 50,
					Duration:  40, // ends at 90
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 50,
					Duration:  100, // ends at 150
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 100, // adjusted to parent start
					Duration:  50,  // adjusted duration
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 50,
					Duration:  200, // starts at 50, ends at 250
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
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
					SpanID:    [8]byte{2},
					StartTime: 100, // adjusted to parent start
					Duration:  100, // adjusted to parent duration
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
				},
			},
		},
		{
			name: "child with dropped parent - should be dropped",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 100,
					Duration:  50,
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}}, // parent doesn't exist
					},
				},
			},
			expected: map[pcommon.SpanID]CPSpan{}, // child should be dropped
		},
		{
			name: "span without references",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:     [8]byte{1},
					StartTime:  100,
					Duration:   100,
					References: []CPSpanReference{}, // no references
				},
			},
			expected: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:     [8]byte{1},
					StartTime:  100,
					Duration:   100,
					References: []CPSpanReference{},
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
			for k, v := range tt.input {
				inputCopy[k] = v
			}

			result := sanitizeOverFlowingChildren(inputCopy)
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

func TestSanitizeOverFlowingChildren_ReferenceUpdate(t *testing.T) {
	// Test that references are properly updated with parent span pointers
	input := map[pcommon.SpanID]CPSpan{
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

	result := sanitizeOverFlowingChildren(input)

	// Verify child span's reference has parent span pointer
	childSpan := result[[8]byte{2}]
	assert.NotNil(t, childSpan.References[0].Span, "reference should have parent span pointer")
	assert.Equal(t, pcommon.SpanID([8]byte{1}), childSpan.References[0].Span.SpanID, "reference should point to correct parent")
}
