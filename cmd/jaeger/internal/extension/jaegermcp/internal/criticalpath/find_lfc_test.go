// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func spanID(id byte) pcommon.SpanID {
	return pcommon.SpanID{id}
}

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
				SpanID:       spanID(1),
				ChildSpanIDs: nil,
			},
			expectedSpanID: nil,
		},
		{
			name: "single child without returningChildStartTime",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 120,
					Duration:  30,
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2)},
			},
			expectedSpanID: &pcommon.SpanID{2},
		},
		{
			name: "multiple children returns last finishing",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 110,
					Duration:  20, // ends at 130
				},
				spanID(3): {
					SpanID:    spanID(3),
					StartTime: 140,
					Duration:  40, // ends at 180
				},
				spanID(4): {
					SpanID:    spanID(4),
					StartTime: 120,
					Duration:  30, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3), spanID(4)},
			},
			expectedSpanID: &pcommon.SpanID{3}, // ends at 180, latest
		},
		{
			name: "with returningChildStartTime finds child finishing just before",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 110,
					Duration:  20, // ends at 130
				},
				spanID(3): {
					SpanID:    spanID(3),
					StartTime: 140,
					Duration:  40, // ends at 180
				},
				spanID(4): {
					SpanID:    spanID(4),
					StartTime: 120,
					Duration:  30, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3), spanID(4)},
			},
			returningChildStartTime: new(uint64(160)),
			expectedSpanID:          &pcommon.SpanID{4}, // ends at 150 < 160, latest before returningChildStartTime
		},
		{
			name: "with returningChildStartTime no child finishes before",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 110,
					Duration:  50, // ends at 160
				},
				spanID(3): {
					SpanID:    spanID(3),
					StartTime: 140,
					Duration:  40, // ends at 180
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3)},
			},
			returningChildStartTime: new(uint64(150)),
			expectedSpanID:          nil, // no child ends before 150
		},
		{
			name: "child missing from spanMap is skipped",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 120,
					Duration:  30,
				},
				// spanID(3) is missing from the map
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3)},
			},
			expectedSpanID: &pcommon.SpanID{2},
		},
		{
			name:    "all children missing from spanMap",
			spanMap: map[pcommon.SpanID]CPSpan{},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3)},
			},
			expectedSpanID: nil,
		},
		{
			name: "with returningChildStartTime selects best among multiple valid children",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 100,
					Duration:  20, // ends at 120
				},
				spanID(3): {
					SpanID:    spanID(3),
					StartTime: 110,
					Duration:  25, // ends at 135
				},
				spanID(4): {
					SpanID:    spanID(4),
					StartTime: 130,
					Duration:  20, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3), spanID(4)},
			},
			returningChildStartTime: new(uint64(155)),
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
