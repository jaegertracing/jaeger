// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"slices"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// removeOverflowingChildren removes or adjusts child spans that overflow their parent's time range.
// An overflowing child span is one whose time range falls outside its parent span's time range.
// The function adjusts the start time and duration of overflowing child spans
// to ensure they fit within the time range of their parent span.
//
// This function uses deterministic DFS traversal to ensure consistent results across runs.
// It processes parents before children to ensure cascading removals are deterministic.
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	if len(spanMap) == 0 {
		return spanMap
	}

	// Track which spans should be dropped (deterministically).
	droppedSpans := make(map[pcommon.SpanID]bool)
	visited := make(map[pcommon.SpanID]bool)

	// Build deterministic IDs list.
	spanIDs := make([]pcommon.SpanID, 0, len(spanMap))
	for spanID := range spanMap {
		spanIDs = append(spanIDs, spanID)
	}
	sortSpanIDs(spanIDs)

	// Build adjacency from ParentSpanID as source of truth.
	childrenByParent := make(map[pcommon.SpanID][]pcommon.SpanID, len(spanMap))
	rootSpans := make([]pcommon.SpanID, 0)
	for _, spanID := range spanIDs {
		span := spanMap[spanID]
		if span.ParentSpanID.IsEmpty() {
			rootSpans = append(rootSpans, spanID)
			continue
		}
		childrenByParent[span.ParentSpanID] = append(childrenByParent[span.ParentSpanID], spanID)
	}
	for parentID := range childrenByParent {
		sortSpanIDs(childrenByParent[parentID])
	}

	// Drop orphaned subtrees first.
	for _, spanID := range spanIDs {
		span := spanMap[spanID]
		if span.ParentSpanID.IsEmpty() {
			continue
		}
		if _, parentExists := spanMap[span.ParentSpanID]; !parentExists {
			markSubtreeDropped(spanID, droppedSpans, childrenByParent)
		}
	}

	// DFS traversal: parents before children, deterministic.
	var dfs func(spanID pcommon.SpanID)
	dfs = func(spanID pcommon.SpanID) {
		if visited[spanID] {
			return
		}
		visited[spanID] = true

		span, ok := spanMap[spanID]
		if !ok || droppedSpans[spanID] {
			return
		}

		if !span.ParentSpanID.IsEmpty() {
			parentSpan, parentExists := spanMap[span.ParentSpanID]
			if !parentExists || droppedSpans[span.ParentSpanID] {
				markSubtreeDropped(spanID, droppedSpans, childrenByParent)
				return
			}
			if shouldDropOverflowingChild(span, parentSpan) {
				markSubtreeDropped(spanID, droppedSpans, childrenByParent)
				removeChildFromParent(span.SpanID, parentSpan, spanMap)
				return
			}
			adjustChild(span, parentSpan, spanMap)
		}

		for _, childID := range childrenByParent[spanID] {
			dfs(childID)
		}
	}

	for _, rootID := range rootSpans {
		dfs(rootID)
	}
	// Ensure deterministic coverage for disconnected / malformed graphs too.
	for _, spanID := range spanIDs {
		dfs(spanID)
	}

	// Build result: keep only non-dropped spans
	result := make(map[pcommon.SpanID]CPSpan)
	for spanID, span := range spanMap {
		if !droppedSpans[spanID] {
			result[spanID] = span
		}
	}

	return result
}

// markSubtreeDropped marks a span and all its descendants as dropped.
func markSubtreeDropped(
	spanID pcommon.SpanID,
	droppedSpans map[pcommon.SpanID]bool,
	childrenByParent map[pcommon.SpanID][]pcommon.SpanID,
) {
	stack := []pcommon.SpanID{spanID}
	for len(stack) > 0 {
		n := len(stack) - 1
		current := stack[n]
		stack = stack[:n]
		if droppedSpans[current] {
			continue
		}
		droppedSpans[current] = true
		stack = append(stack, childrenByParent[current]...)
	}
}

// shouldDropOverflowingChild determines if a child span overflows its parent and should be dropped.
func shouldDropOverflowingChild(child, parent CPSpan) bool {
	childEndTime := child.StartTime + child.Duration
	parentEndTime := parent.StartTime + parent.Duration

	// Child is completely after parent
	if child.StartTime >= parentEndTime {
		return true
	}

	// Child is completely before parent
	if childEndTime <= parent.StartTime {
		return true
	}

	return false
}

// adjustChild truncates a child span to fit within its parent's bounds.
func adjustChild(child, parent CPSpan, spanMap map[pcommon.SpanID]CPSpan) {
	childEndTime := child.StartTime + child.Duration
	parentEndTime := parent.StartTime + parent.Duration

	if child.StartTime >= parent.StartTime {
		if childEndTime > parentEndTime {
			// child end after parent, truncate is needed
			//      |----parent----|
			//              |----child--|
			child.Duration = parentEndTime - child.StartTime
			spanMap[child.SpanID] = child
		}
		// else: everything looks good
	} else {
		// child starts before parent
		if childEndTime <= parentEndTime {
			// child start before parent, truncate is needed
			//      |----parent----|
			//   |----child--|
			child.StartTime = parent.StartTime
			child.Duration = childEndTime - parent.StartTime
			spanMap[child.SpanID] = child
		} else {
			// child start before parent and end after parent, truncate is needed
			//      |----parent----|
			//  |---------child---------|
			child.StartTime = parent.StartTime
			child.Duration = parentEndTime - parent.StartTime
			spanMap[child.SpanID] = child
		}
	}
}

// removeChildFromParent removes a child span ID from its parent's ChildSpanIDs list.
func removeChildFromParent(childID pcommon.SpanID, parent CPSpan, spanMap map[pcommon.SpanID]CPSpan) {
	filteredChildren := make([]pcommon.SpanID, 0, len(parent.ChildSpanIDs))
	for _, id := range parent.ChildSpanIDs {
		if id != childID {
			filteredChildren = append(filteredChildren, id)
		}
	}
	parent.ChildSpanIDs = filteredChildren
	spanMap[parent.SpanID] = parent
}

// sortSpanIDs sorts span IDs in ascending order for deterministic traversal.
func sortSpanIDs(ids []pcommon.SpanID) {
	slices.SortFunc(ids, compareSpanIDs)
}

// compareSpanIDs compares two span IDs lexicographically.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareSpanIDs(a, b pcommon.SpanID) int {
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
