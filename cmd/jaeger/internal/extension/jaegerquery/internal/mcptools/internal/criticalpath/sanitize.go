// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"bytes"
	"cmp"
	"slices"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// removeOverflowingChildren removes or adjusts child spans that overflow their parent's time range.
// An overflowing child span is one whose time range falls outside its parent span's time range.
// The function adjusts the start time and duration of overflowing child spans
// to ensure they fit within the time range of their parent span.
//
// The traversal is a top-down BFS from every root span so that a parent is
// always resolved (kept or dropped) before any of its children are evaluated.
// When a span is dropped, its children are enqueued for unconditional deletion
// in the same pass, guaranteeing the cascade is order-independent regardless
// of the underlying map's iteration order.
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	// Seed the BFS queue with every root span (spans that have no parent in the
	// map). Roots are always kept; we only evaluate children against their parent.
	queue := make([]pcommon.SpanID, 0, len(spanMap))
	for spanID, span := range spanMap {
		_, parentInMap := spanMap[span.ParentSpanID]
		if span.ParentSpanID.IsEmpty() || !parentInMap {
			// True root (no parent) or orphan whose parent is missing from the map.
			queue = append(queue, spanID)
		}
	}

	// Deterministic ordering: sort initial seeds by StartTime, tie-breaking by SpanID bytes.
	// This ensures cycle breaking and traversal order are 100% deterministic regardless of
	// Go's randomized map iteration order.
	slices.SortFunc(queue, func(a, b pcommon.SpanID) int {
		spanA := spanMap[a]
		spanB := spanMap[b]
		if spanA.StartTime != spanB.StartTime {
			return cmp.Compare(spanA.StartTime, spanB.StartTime)
		}
		return bytes.Compare(a[:], b[:])
	})

	// Track enqueued spans using a zero-allocation map[SpanID]struct{} set.
	// Marking spans as enqueued at insertion time guarantees that each span is
	// enqueued at most once and prevents duplicate BFS queue growth.
	enqueued := make(map[pcommon.SpanID]struct{}, len(spanMap))
	for _, id := range queue {
		enqueued[id] = struct{}{}
	}

	for len(queue) > 0 {
		spanID := queue[0]
		queue = queue[1:]

		span, ok := spanMap[spanID]
		if !ok {
			// Already deleted by a prior cascade step.
			continue
		}

		// Spans whose parent is not in the map (true orphans) are dropped.
		// True roots have an empty ParentSpanID and are always kept.
		if !span.ParentSpanID.IsEmpty() {
			if _, parentExists := spanMap[span.ParentSpanID]; !parentExists {
				// Drop this orphan and cascade deletion to all its descendants.
				delete(spanMap, spanID)
				for _, childID := range span.ChildSpanIDs {
					if _, seen := enqueued[childID]; !seen {
						enqueued[childID] = struct{}{}
						queue = append(queue, childID)
					}
				}
				continue
			}
		}

		// Enqueue this span's children so they are processed after the parent.
		// Exclude already-enqueued spans to break cycles and prevent infinite loops.
		hasCycle := false
		validChildren := make([]pcommon.SpanID, 0, len(span.ChildSpanIDs))
		for _, childID := range span.ChildSpanIDs {
			if _, seen := enqueued[childID]; seen {
				hasCycle = true
				continue
			}
			enqueued[childID] = struct{}{}
			validChildren = append(validChildren, childID)
		}

		if hasCycle {
			// Prune cyclic edges in-place so downstream callers (e.g. computeCriticalPath)
			// cannot follow the back-edge into a stack overflow.
			span.ChildSpanIDs = validChildren
			spanMap[spanID] = span
		}
		queue = appendChildren(queue, validChildren)

		// Root spans have no parent to overflow; nothing to check.
		if span.ParentSpanID.IsEmpty() {
			continue
		}

		// At this point the parent is guaranteed to be in spanMap and already
		// resolved (clamped or kept) because BFS processes parents first.
		parentSpan := spanMap[span.ParentSpanID]

		childEndTime := span.StartTime + span.Duration
		parentEndTime := parentSpan.StartTime + parentSpan.Duration

		if span.StartTime >= parentSpan.StartTime {
			if span.StartTime >= parentEndTime {
				// child outside of parent range => drop the child span
				//      |----parent----|
				//                        |----child--|
				delete(spanMap, span.SpanID)
				cascadeDelete(spanMap, span.ChildSpanIDs)

				// Remove the childSpanId from its parent span
				spanMap[parentSpan.SpanID] = removeChild(parentSpan, span.SpanID)
				continue
			}
			if childEndTime > parentEndTime {
				// child end after parent, truncate is needed
				//      |----parent----|
				//              |----child--|
				span.Duration = parentEndTime - span.StartTime
				spanMap[span.SpanID] = span
				continue
			}
			// everything looks good
			// |----parent----|
			//   |----child--|
			continue
		}

		switch {
		case childEndTime <= parentSpan.StartTime:
			// child outside of parent range => drop the child span
			//                      |----parent----|
			//       |----child--|
			delete(spanMap, span.SpanID)
			cascadeDelete(spanMap, span.ChildSpanIDs)

			// Remove the childSpanId from its parent span
			spanMap[parentSpan.SpanID] = removeChild(parentSpan, span.SpanID)
		case childEndTime <= parentEndTime:
			// child start before parent, truncate is needed
			//      |----parent----|
			//   |----child--|
			span.StartTime = parentSpan.StartTime
			span.Duration = childEndTime - parentSpan.StartTime
			spanMap[span.SpanID] = span
		default:
			// child start before parent and end after parent, truncate is needed
			//      |----parent----|
			//  |---------child---------|
			span.StartTime = parentSpan.StartTime
			span.Duration = parentEndTime - parentSpan.StartTime
			spanMap[span.SpanID] = span
		}
	}

	// Any spans that were never enqueued have no path to a root or orphan seed;
	// they form isolated closed cycles with no root. Purge them.
	for id := range spanMap {
		if _, seen := enqueued[id]; !seen {
			delete(spanMap, id)
		}
	}

	return spanMap
}

// removeChild returns a copy of parent with childID removed from ChildSpanIDs.
func removeChild(parent CPSpan, childID pcommon.SpanID) CPSpan {
	filtered := make([]pcommon.SpanID, 0, len(parent.ChildSpanIDs))
	for _, id := range parent.ChildSpanIDs {
		if id != childID {
			filtered = append(filtered, id)
		}
	}
	parent.ChildSpanIDs = filtered
	return parent
}

// cascadeDelete removes all descendants of a dropped span from spanMap.
// It walks the ChildSpanIDs transitively so that no orphan is left behind.
func cascadeDelete(spanMap map[pcommon.SpanID]CPSpan, children []pcommon.SpanID) {
	queue := make([]pcommon.SpanID, len(children))
	copy(queue, children)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		child, ok := spanMap[id]
		if !ok {
			continue
		}
		delete(spanMap, id)
		queue = appendChildren(queue, child.ChildSpanIDs)
	}
}

// appendChildren appends ids to queue and returns the extended slice.
func appendChildren(queue []pcommon.SpanID, ids []pcommon.SpanID) []pcommon.SpanID {
	return append(queue, ids...)
}
