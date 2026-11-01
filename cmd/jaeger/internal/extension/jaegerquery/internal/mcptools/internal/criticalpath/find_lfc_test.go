// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func spanID(id byte) pcommon.SpanID {
	return pcommon.SpanID{id}
}

func TestChildSpanIterator(t *testing.T) {
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
			name: "back-to-back sequential spans skipped due to strict inequality",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 100,
					Duration:  50, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				StartTime:    100,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{spanID(2)},
			},
			returningChildStartTime: new(uint64(150)),
			expectedSpanID:          &pcommon.SpanID{2}, // Should find span 2 since it ends exactly at 150
		},
		{
			name: "child starting at returning boundary is skipped",
			spanMap: map[pcommon.SpanID]CPSpan{
				spanID(2): {
					SpanID:    spanID(2),
					StartTime: 150,
					Duration:  0,
				},
				spanID(3): {
					SpanID:    spanID(3),
					StartTime: 100,
					Duration:  50, // ends at 150
				},
			},
			currentSpan: CPSpan{
				SpanID:       spanID(1),
				ChildSpanIDs: []pcommon.SpanID{spanID(2), spanID(3)},
			},
			returningChildStartTime: new(uint64(150)),
			expectedSpanID:          &pcommon.SpanID{3},
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
			children := newChildSpanIterator(tt.spanMap, tt.currentSpan)
			result := children.next(tt.returningChildStartTime)

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

func TestChildSpanIterator_TiesAndBoundaries(t *testing.T) {
	parent := CPSpan{ChildSpanIDs: []pcommon.SpanID{{2}, {3}, {4}, {5}, {6}, {7}, {8}}}
	spans := map[pcommon.SpanID]CPSpan{
		{2}: {SpanID: spanID(2), StartTime: 90, Duration: 10},
		{3}: {SpanID: spanID(3), StartTime: 80, Duration: 20}, // same end, later in input
		{4}: {SpanID: spanID(4), StartTime: 90},               // excluded at the returning boundary
		{5}: {SpanID: spanID(5), StartTime: 70, Duration: 20}, // ends at the boundary
		{6}: {SpanID: spanID(6), StartTime: 60},               // zero duration, before the boundary
		{7}: {SpanID: spanID(7)},                              // end time zero was never selected by the scan
		// Child 8 is missing.
	}
	originalIDs := slices.Clone(parent.ChildSpanIDs)
	children := newChildSpanIterator(spans, parent)
	var boundary *uint64
	for _, id := range []byte{2, 5, 6} {
		child := children.next(boundary)
		require.NotNil(t, child)
		require.Equal(t, spans[spanID(id)], *child)
		boundary = &child.StartTime
	}
	require.Nil(t, children.next(boundary))
	require.Nil(t, children.next(boundary))
	require.Equal(t, originalIDs, parent.ChildSpanIDs)
}

func TestChildSpanIterator_MatchesScan(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	for range 1000 {
		parent := CPSpan{}
		spans := make(map[pcommon.SpanID]CPSpan)
		for i := range 64 {
			id := spanID(byte(i + 1))
			parent.ChildSpanIDs = append(parent.ChildSpanIDs, id)
			if i%10 != 0 {
				spans[id] = CPSpan{SpanID: id, StartTime: random.Uint64N(30), Duration: random.Uint64N(20)}
			}
		}
		random.Shuffle(len(parent.ChildSpanIDs), func(i, j int) {
			parent.ChildSpanIDs[i], parent.ChildSpanIDs[j] = parent.ChildSpanIDs[j], parent.ChildSpanIDs[i]
		})
		children := newChildSpanIterator(spans, parent)
		var boundary *uint64
		for {
			expected := scanLastChild(spans, parent, boundary)
			actual := children.next(boundary)
			require.Equal(t, expected, actual)
			if expected == nil {
				break
			}
			boundary = &expected.StartTime
		}
	}
}

// scanLastChild is a reference implementation of the original full-list search.
func scanLastChild(spans map[pcommon.SpanID]CPSpan, parent CPSpan, boundary *uint64) *CPSpan {
	var selected CPSpan
	var maxEnd uint64
	for _, id := range parent.ChildSpanIDs {
		child, exists := spans[id]
		if !exists {
			continue
		}
		end := child.StartTime + child.Duration
		if boundary != nil && (child.StartTime >= *boundary || end > *boundary) {
			continue
		}
		if end > maxEnd {
			maxEnd = end
			selected = child
		}
	}
	if maxEnd == 0 {
		return nil
	}
	return &selected
}
