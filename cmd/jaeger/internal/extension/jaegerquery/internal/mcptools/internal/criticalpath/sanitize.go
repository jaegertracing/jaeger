// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// removeOverflowingChildren removes or adjusts child spans that overflow their parent's time range.
// An overflowing child span is one whose time range falls outside its parent span's time range.
// The function adjusts the start time and duration of overflowing child spans
// to ensure they fit within the time range of their parent span.
//
// The real observable bug this fixes is a clamping-order hazard: with spans
// A[0..100] -> B[50..150] -> C[120..140], if C is visited before B in a single
// random-order map pass, C passes its own bounds check (120 is inside B's
// original range) and survives. B is then truncated to [50..100], leaving C
// outside its parent. The BFS below eliminates that hazard by guaranteeing that
// every parent is fully resolved before any of its children are evaluated.
//
// Spans in a parent cycle with no root are skipped because they have no path
// to a root span.
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	// Seed the BFS queue with every root span (a span whose parent is absent
	// from the map, including true roots with an empty ParentSpanID).
	queue := make([]pcommon.SpanID, 0, len(spanMap))
	for spanID, span := range spanMap {
		_, parentInMap := spanMap[span.ParentSpanID]
		if span.ParentSpanID.IsEmpty() || !parentInMap {
			queue = append(queue, spanID)
		}
	}

	for len(queue) > 0 {
		// Dequeue the next span to process.
		spanID := queue[0]
		queue = queue[1:]

		span, ok := spanMap[spanID]
		if !ok {
			// Already deleted by a prior cascade step.
			continue
		}

		// Spans whose parent has been deleted (dropped orphans) are removed here.
		// True roots have an empty ParentSpanID and are always kept.
		if !span.ParentSpanID.IsEmpty() {
			if _, parentExists := spanMap[span.ParentSpanID]; !parentExists {
				// Drop this orphan and cascade deletion to all its descendants.
				delete(spanMap, spanID)
				queue = append(queue, span.ChildSpanIDs...)
				continue
			}
		}

		// Enqueue this span's children so they are processed after the parent.
		queue = append(queue, span.ChildSpanIDs...)

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
				// Children of the dropped span are already enqueued; they will
				// be caught by the orphan-detection branch above.
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
			// Children of the dropped span are already enqueued; they will
			// be caught by the orphan-detection branch above.
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
