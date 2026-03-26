// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestFindLastFinishingChildSpan(t *testing.T) {
	tests := []struct {
		name                    string
		spanMap                 map[pcommon.SpanID]CPSpan
		currentSpan             CPSpan
		returningChildStartTime *uint64
		expectedSpanID          *pcommon.SpanID
	}{
		{
			name:    "no children",
			spanMap: map[pcommon.SpanID]CPSpan{},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				ChildSpanIDs: nil,
			},
			expectedSpanID: nil,
		},
		{
			name: "single child without returningChildStartTime",
			spanMap: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 120,
					Duration:  30,
				},
			},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}},
			},
			expectedSpanID: &pcommon.SpanID{2},
		},
		{
			name: "multiple children returns last finishing",
			spanMap: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 110,
					Duration:  20, // ends at 130
				},
				[8]byte{3}: {
					SpanID:    [8]byte{3},
					StartTime: 140,
					Duration:  40, // ends at 180
				},
				[8]byte{4}: {
					SpanID:    [8]byte{4},
					StartTime: 120,
					Duration:  30, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}, [8]byte{4}},
			},
			expectedSpanID: &pcommon.SpanID{3}, // ends at 180, latest
		},
		{
			name: "with returningChildStartTime finds child finishing just before",
			spanMap: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 110,
					Duration:  20, // ends at 130
				},
				[8]byte{3}: {
					SpanID:    [8]byte{3},
					StartTime: 140,
					Duration:  40, // ends at 180
				},
				[8]byte{4}: {
					SpanID:    [8]byte{4},
					StartTime: 120,
					Duration:  30, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}, [8]byte{4}},
			},
			returningChildStartTime: ptr(uint64(160)),
			expectedSpanID:          &pcommon.SpanID{4}, // ends at 150 < 160, latest before returningChildStartTime
		},
		{
			name: "with returningChildStartTime no child finishes before",
			spanMap: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 110,
					Duration:  50, // ends at 160
				},
				[8]byte{3}: {
					SpanID:    [8]byte{3},
					StartTime: 140,
					Duration:  40, // ends at 180
				},
			},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}},
			},
			returningChildStartTime: ptr(uint64(150)),
			expectedSpanID:          nil, // no child ends before 150
		},
		{
			name: "child missing from spanMap is skipped",
			spanMap: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 120,
					Duration:  30,
				},
				// [8]byte{3} is missing from the map
			},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}},
			},
			expectedSpanID: &pcommon.SpanID{2},
		},
		{
			name:    "all children missing from spanMap",
			spanMap: map[pcommon.SpanID]CPSpan{},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}},
			},
			expectedSpanID: nil,
		},
		{
			name: "with returningChildStartTime selects best among multiple valid children",
			spanMap: map[pcommon.SpanID]CPSpan{
				[8]byte{2}: {
					SpanID:    [8]byte{2},
					StartTime: 100,
					Duration:  20, // ends at 120
				},
				[8]byte{3}: {
					SpanID:    [8]byte{3},
					StartTime: 110,
					Duration:  25, // ends at 135
				},
				[8]byte{4}: {
					SpanID:    [8]byte{4},
					StartTime: 130,
					Duration:  20, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       [8]byte{1},
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{[8]byte{2}, [8]byte{3}, [8]byte{4}},
			},
			returningChildStartTime: ptr(uint64(155)),
			expectedSpanID:          &pcommon.SpanID{4}, // ends at 150 < 155, closest to returningChildStartTime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLastFinishingChildSpan(tt.spanMap, tt.currentSpan, tt.returningChildStartTime)

			if tt.expectedSpanID == nil {
				assert.Nil(t, result, "expected nil result")
			} else {
				assert.NotNil(t, result, "expected non-nil result")
				if result != nil {
					assert.Equal(t, *tt.expectedSpanID, result.SpanID, "unexpected span ID")
				}
			}
		})
	}
}

func ptr(v uint64) *uint64 {
	return &v
}
