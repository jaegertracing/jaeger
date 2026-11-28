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
// Each span is checked against its parent's already adjusted range, so spans are
// visited parents first (see spanIDsParentsFirst).
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	for _, spanID := range spanIDsParentsFirst(spanMap) {
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

// spanIDsParentsFirst returns the IDs in spanMap ordered so that every span comes
// after its parent: a breadth-first walk from the spans without a parent, then any
// span the walk does not reach, such as one whose parent is not in the trace.
func spanIDsParentsFirst(spanMap map[pcommon.SpanID]CPSpan) []pcommon.SpanID {
	ordered := make([]pcommon.SpanID, 0, len(spanMap))
	visited := make(map[pcommon.SpanID]struct{}, len(spanMap))
	for spanID, span := range spanMap {
		if span.ParentSpanID.IsEmpty() {
			ordered = append(ordered, spanID)
			visited[spanID] = struct{}{}
		}
	}
	for i := 0; i < len(ordered); i++ {
		for _, childID := range spanMap[ordered[i]].ChildSpanIDs {
			if _, ok := spanMap[childID]; !ok {
				continue
			}
			if _, ok := visited[childID]; ok {
				continue
			}
			ordered = append(ordered, childID)
			visited[childID] = struct{}{}
		}
	}
	for spanID := range spanMap {
		if _, ok := visited[spanID]; !ok {
			ordered = append(ordered, spanID)
		}
	}
	return ordered
}
