// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"bytes"
	"slices"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// removeOverflowingChildren removes or adjusts child spans that overflow their parent's time range.
// An overflowing child span is one whose time range falls outside its parent span's time range.
// The function adjusts the start time and duration of overflowing child spans
// to ensure they fit within the time range of their parent span.
//
// Spans are visited parent-first (see ancestorFirstOrder). A span is only
// compared against its direct parent, so truncating a parent narrows the bounds
// of its whole subtree: children, and grandchildren below them, have to be
// visited after the parent has reached its final bounds.
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	for _, spanID := range ancestorFirstOrder(spanMap) {
		span, ok := spanMap[spanID]
		if !ok || span.ParentSpanID.IsEmpty() {
			continue
		}

		// parentSpan will be undefined when its parent was dropped previously
		parentSpan, parentExists := spanMap[span.ParentSpanID]
		if !parentExists {
			// Drop the child spans of dropped parent span
			delete(spanMap, span.SpanID)
			continue
		}

		childEndTime := span.StartTime + span.Duration
		parentEndTime := parentSpan.StartTime + parentSpan.Duration

		if span.StartTime >= parentSpan.StartTime {
			if span.StartTime >= parentEndTime {
				// child outside of parent range => drop the child span
				//      |----parent----|
				//                        |----child--|
				delete(spanMap, span.SpanID)

				// Remove the childSpanId from its parent span
				filteredChildren := make([]pcommon.SpanID, 0, len(parentSpan.ChildSpanIDs))
				for _, childID := range parentSpan.ChildSpanIDs {
					if childID != span.SpanID {
						filteredChildren = append(filteredChildren, childID)
					}
				}
				parentSpan.ChildSpanIDs = filteredChildren
				spanMap[parentSpan.SpanID] = parentSpan
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

			// Remove the childSpanId from its parent span
			filteredChildren := make([]pcommon.SpanID, 0, len(parentSpan.ChildSpanIDs))
			for _, childID := range parentSpan.ChildSpanIDs {
				if childID != span.SpanID {
					filteredChildren = append(filteredChildren, childID)
				}
			}
			parentSpan.ChildSpanIDs = filteredChildren
			spanMap[parentSpan.SpanID] = parentSpan
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

// ancestorFirstOrder lists the span IDs of spanMap so that no span precedes
// its parent. Spans whose parent is absent from the map start a tree of their
// own; the caller drops them, and visiting them first lets their descendants
// be dropped by the same rule.
//
// Span IDs are compared to keep the order stable: iterating the map directly
// would be randomized, which is what made the previous single pass return a
// different result on every call.
//
// A trace whose parent links form a cycle is malformed, but the traversal must
// still terminate, so unreachable spans are appended last.
func ancestorFirstOrder(spanMap map[pcommon.SpanID]CPSpan) []pcommon.SpanID {
	children := make(map[pcommon.SpanID][]pcommon.SpanID, len(spanMap))
	roots := make([]pcommon.SpanID, 0, len(spanMap))
	for spanID, span := range spanMap {
		if _, ok := spanMap[span.ParentSpanID]; !ok {
			roots = append(roots, spanID)
			continue
		}
		children[span.ParentSpanID] = append(children[span.ParentSpanID], spanID)
	}
	sortSpanIDs(roots)
	for _, childIDs := range children {
		sortSpanIDs(childIDs)
	}

	order := make([]pcommon.SpanID, 0, len(spanMap))
	visited := make(map[pcommon.SpanID]struct{}, len(spanMap))
	queue := roots
	for len(queue) > 0 {
		spanID := queue[0]
		queue = queue[1:]
		if _, ok := visited[spanID]; ok {
			continue
		}
		visited[spanID] = struct{}{}
		order = append(order, spanID)
		queue = append(queue, children[spanID]...)
	}

	leftover := make([]pcommon.SpanID, 0, len(spanMap)-len(order))
	for spanID := range spanMap {
		if _, ok := visited[spanID]; !ok {
			leftover = append(leftover, spanID)
		}
	}
	sortSpanIDs(leftover)
	return append(order, leftover...)
}

func sortSpanIDs(spanIDs []pcommon.SpanID) {
	slices.SortFunc(spanIDs, func(a, b pcommon.SpanID) int {
		return bytes.Compare(a[:], b[:])
	})
}
