// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestGetChildOfSpans(t *testing.T) {
	tests := []struct {
		name     string
		input    map[pcommon.SpanID]CPSpan
		expected int // expected number of spans after filtering
	}{
		{
			name: "no FOLLOWS_FROM spans",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID: [8]byte{1},
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{0}},
					},
				},
				[8]byte{2}: {
					SpanID: [8]byte{2},
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
				},
			},
			expected: 2,
		},
		{
			name: "single FOLLOWS_FROM span",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID: [8]byte{2},
					References: []CPSpanReference{
						{RefType: "FOLLOWS_FROM", SpanID: [8]byte{1}},
					},
				},
			},
			expected: 1, // span 2 should be removed
		},
		{
			name: "FOLLOWS_FROM span with children",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{3}},
					References: []CPSpanReference{
						{RefType: "FOLLOWS_FROM", SpanID: [8]byte{1}},
					},
				},
				[8]byte{3}: {
					SpanID: [8]byte{3},
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{2}},
					},
				},
			},
			expected: 1, // spans 2 and 3 should be removed
		},
		{
			name: "mixed CHILD_OF and FOLLOWS_FROM",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}},
				},
				[8]byte{2}: {
					SpanID: [8]byte{2},
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{1}},
					},
				},
				[8]byte{3}: {
					SpanID: [8]byte{3},
					References: []CPSpanReference{
						{RefType: "FOLLOWS_FROM", SpanID: [8]byte{1}},
					},
				},
			},
			expected: 2, // only span 3 should be removed
		},
		{
			name: "nested FOLLOWS_FROM descendants",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{1}: {
					SpanID:       [8]byte{1},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
				},
				[8]byte{2}: {
					SpanID:       [8]byte{2},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{3}},
					References: []CPSpanReference{
						{RefType: "FOLLOWS_FROM", SpanID: [8]byte{1}},
					},
				},
				[8]byte{3}: {
					SpanID:       [8]byte{3},
					ChildSpanIDs: []pcommon.SpanID{[8]byte{4}},
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{2}},
					},
				},
				[8]byte{4}: {
					SpanID: [8]byte{4},
					References: []CPSpanReference{
						{RefType: "CHILD_OF", SpanID: [8]byte{3}},
					},
				},
			},
			expected: 1, // spans 2, 3, 4 should be removed (all descendants of FOLLOWS_FROM)
		},
		{
			name:     "empty span map",
			input:    map[pcommon.SpanID]CPSpan{},
			expected: 0,
		},
		{
			name: "FOLLOWS_FROM without parent in map",
			input: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID: [8]byte{2},
					References: []CPSpanReference{
						{RefType: "FOLLOWS_FROM", SpanID: [8]byte{1}}, // parent doesn't exist
					},
				},
			},
			expected: 0, // span should still be removed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data
			inputCopy := make(map[pcommon.SpanID]CPSpan)
			maps.Copy(inputCopy, tt.input)

			result := getChildOfSpans(inputCopy)
			assert.Len(t, result, tt.expected, "unexpected number of spans after filtering")

			// Verify FOLLOWS_FROM spans are removed
			for _, span := range result {
				if len(span.References) > 0 {
					assert.NotEqual(t, "FOLLOWS_FROM", span.References[0].RefType,
						"FOLLOWS_FROM span should be removed")
				}
			}
		})
	}
}
