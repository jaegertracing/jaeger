// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestSanitizeOverFlowingChildren_TruncatesGrandchild(t *testing.T) {
	// The middle span overflows the root and is truncated from 10..210 to
	// 10..100; the leaf still fits the original 10..210, so it has to be
	// truncated against the tightened bounds of its parent.
	rootID := pcommon.SpanID{1}
	midID := pcommon.SpanID{2}
	leafID := pcommon.SpanID{3}

	input := map[pcommon.SpanID]CPSpan{
		rootID: {
			SpanID:       rootID,
			StartTime:    0,
			Duration:     100,
			ChildSpanIDs: []pcommon.SpanID{midID},
		},
		midID: {
			SpanID:       midID,
			ParentSpanID: rootID,
			StartTime:    10,
			Duration:     200,
			ChildSpanIDs: []pcommon.SpanID{leafID},
		},
		leafID: {
			SpanID:       leafID,
			ParentSpanID: midID,
			StartTime:    50,
			Duration:     100,
		},
	}

	result := removeOverflowingChildren(input)

	assert.Equal(t, uint64(90), result[midID].Duration, "middle span should be truncated to the root")
	assert.Equal(t, uint64(50), result[leafID].StartTime)
	assert.Equal(t, uint64(50), result[leafID].Duration, "leaf span should be truncated to its parent")
}

func TestSanitizeOverFlowingChildren_IsOrderIndependent(t *testing.T) {
	// Go randomizes map iteration, so a single pass that visits a grandchild
	// before its parent leaves the grandchild overflowing on some runs only.
	// The invariant of the function must hold on every run.
	rootID := pcommon.SpanID{1}
	midID := pcommon.SpanID{2}
	leafID := pcommon.SpanID{3}

	for range 100 {
		spanMap := map[pcommon.SpanID]CPSpan{
			rootID: {
				SpanID:       rootID,
				StartTime:    0,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{midID},
			},
			midID: {
				SpanID:       midID,
				ParentSpanID: rootID,
				StartTime:    10,
				Duration:     200,
				ChildSpanIDs: []pcommon.SpanID{leafID},
			},
			leafID: {
				SpanID:       leafID,
				ParentSpanID: midID,
				StartTime:    50,
				Duration:     100,
			},
		}

		result := removeOverflowingChildren(spanMap)
		for spanID, span := range result {
			parent, ok := result[span.ParentSpanID]
			if !ok {
				continue
			}
			assert.GreaterOrEqual(t, span.StartTime, parent.StartTime,
				"span %v starts before its parent %v", spanID, span.ParentSpanID)
			assert.LessOrEqual(t, span.StartTime+span.Duration, parent.StartTime+parent.Duration,
				"span %v ends after its parent %v", spanID, span.ParentSpanID)
		}
	}
}

func TestSanitizeOverFlowingChildren_CyclicParentsTerminate(t *testing.T) {
	// Two spans that name each other as parent are unreachable from any root.
	// They are dropped, and the traversal must not loop forever.
	firstID := pcommon.SpanID{1}
	secondID := pcommon.SpanID{2}

	input := map[pcommon.SpanID]CPSpan{
		firstID: {
			SpanID:       firstID,
			ParentSpanID: secondID,
			StartTime:    0,
			Duration:     10,
		},
		secondID: {
			SpanID:       secondID,
			ParentSpanID: firstID,
			StartTime:    10,
			Duration:     10,
		},
	}

	assert.Empty(t, removeOverflowingChildren(input))
}

func TestComputeCriticalPath_IgnoresInvertedSections(t *testing.T) {
	// A child that starts after its parent ends used to produce a section with
	// SectionStart greater than SectionEnd. The handlers subtract SectionStart
	// from SectionEnd as uint64, so such a section reported a SelfTimeUs close
	// to math.MaxUint64 instead of being omitted.
	rootID := pcommon.SpanID{1}
	childID := pcommon.SpanID{2}

	spanMap := map[pcommon.SpanID]CPSpan{
		rootID: {
			SpanID:       rootID,
			StartTime:    0,
			Duration:     100,
			ChildSpanIDs: []pcommon.SpanID{childID},
		},
		childID: {
			SpanID:       childID,
			ParentSpanID: rootID,
			StartTime:    150,
			Duration:     50,
		},
	}

	sections := computeCriticalPath(spanMap, rootID, []Section{}, nil)

	assert.NotEmpty(t, sections)
	for _, section := range sections {
		assert.LessOrEqual(t, section.SectionStart, section.SectionEnd,
			"section of span %v is inverted", section.SpanID)
	}
}
