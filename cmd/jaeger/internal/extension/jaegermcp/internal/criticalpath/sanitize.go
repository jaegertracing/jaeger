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
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	// Get all span IDs
	spanIDs := make([]pcommon.SpanID, 0, len(spanMap))
	for spanID := range spanMap {
		spanIDs = append(spanIDs, spanID)
	}

	for _, spanID := range spanIDs {
		span, ok := spanMap[spanID]
		if !ok || len(span.References) == 0 {
			continue
		}

		// parentSpan will be undefined when its parent was dropped previously
		parentSpan, parentExists := spanMap[span.References[0].SpanID]
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

	// Updated spanIds to ensure not to include dropped spans
	spanIDs = make([]pcommon.SpanID, 0, len(spanMap))
	for spanID := range spanMap {
		spanIDs = append(spanIDs, spanID)
	}

	// Update Child Span References with updated parent span
	for _, spanID := range spanIDs {
		span := spanMap[spanID]
		if len(span.References) > 0 {
			if parentSpan, ok := spanMap[span.References[0].SpanID]; ok {
				span.References[0].Span = &parentSpan
				spanMap[spanID] = span
			}
		}
	}

	return spanMap
}
