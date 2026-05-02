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
	rootSpanIDs := make([]pcommon.SpanID, 0, len(spanMap))
	missingParentSpanIDs := make([]pcommon.SpanID, 0)

	for spanID, span := range spanMap {
		if len(span.References) == 0 {
			rootSpanIDs = append(rootSpanIDs, spanID)
		} else if _, parentExists := spanMap[span.References[0].SpanID]; !parentExists {
			missingParentSpanIDs = append(missingParentSpanIDs, spanID)
		}
	}

	for _, spanID := range missingParentSpanIDs {
		deleteSpanAndDescendants(spanMap, spanID)
	}

	for _, spanID := range rootSpanIDs {
		sanitizeDescendants(spanMap, spanID)
	}

	// Update Child Span References with updated parent span.
	for spanID, span := range spanMap {
		if len(span.References) > 0 {
			if parentSpan, ok := spanMap[span.References[0].SpanID]; ok {
				span.References[0].Span = &parentSpan
				spanMap[spanID] = span
			}
		}
	}

	return spanMap
}

func sanitizeDescendants(spanMap map[pcommon.SpanID]CPSpan, parentSpanID pcommon.SpanID) {
	parentSpan, ok := spanMap[parentSpanID]
	if !ok {
		return
	}

	childSpanIDs := append([]pcommon.SpanID(nil), parentSpan.ChildSpanIDs...)
	for _, childSpanID := range childSpanIDs {
		childSpan, ok := spanMap[childSpanID]
		if !ok {
			continue
		}

		parentSpan = spanMap[parentSpanID]
		sanitizedChildSpan, keepChild := fitSpanWithinParent(childSpan, parentSpan)
		if !keepChild {
			deleteSpanAndDescendants(spanMap, childSpanID)
			removeChildSpanID(spanMap, parentSpanID, childSpanID)
			continue
		}

		spanMap[childSpanID] = sanitizedChildSpan
		sanitizeDescendants(spanMap, childSpanID)
	}
}

func fitSpanWithinParent(span CPSpan, parentSpan CPSpan) (CPSpan, bool) {
	childEndTime := span.StartTime + span.Duration
	parentEndTime := parentSpan.StartTime + parentSpan.Duration

	if span.StartTime >= parentSpan.StartTime {
		if span.StartTime >= parentEndTime {
			// child outside of parent range => drop the child span
			//      |----parent----|
			//                        |----child--|
			return span, false
		}
		if childEndTime > parentEndTime {
			// child end after parent, truncate is needed
			//      |----parent----|
			//              |----child--|
			span.Duration = parentEndTime - span.StartTime
		}

		// everything looks good
		// |----parent----|
		//   |----child--|
		return span, true
	}

	switch {
	case childEndTime <= parentSpan.StartTime:
		// child outside of parent range => drop the child span
		//                      |----parent----|
		//       |----child--|
		return span, false
	case childEndTime <= parentEndTime:
		// child start before parent, truncate is needed
		//      |----parent----|
		//   |----child--|
		span.StartTime = parentSpan.StartTime
		span.Duration = childEndTime - parentSpan.StartTime
	default:
		// child start before parent and end after parent, truncate is needed
		//      |----parent----|
		//  |---------child---------|
		span.StartTime = parentSpan.StartTime
		span.Duration = parentEndTime - parentSpan.StartTime
	}

	return span, true
}

func deleteSpanAndDescendants(spanMap map[pcommon.SpanID]CPSpan, spanID pcommon.SpanID) {
	span, ok := spanMap[spanID]
	if !ok {
		return
	}

	for _, childSpanID := range span.ChildSpanIDs {
		deleteSpanAndDescendants(spanMap, childSpanID)
	}
	delete(spanMap, spanID)
}

func removeChildSpanID(spanMap map[pcommon.SpanID]CPSpan, parentSpanID pcommon.SpanID, childSpanID pcommon.SpanID) {
	parentSpan, ok := spanMap[parentSpanID]
	if !ok {
		return
	}

	filteredChildren := make([]pcommon.SpanID, 0, len(parentSpan.ChildSpanIDs))
	for _, currentChildSpanID := range parentSpan.ChildSpanIDs {
		if currentChildSpanID != childSpanID {
			filteredChildren = append(filteredChildren, currentChildSpanID)
		}
	}
	parentSpan.ChildSpanIDs = filteredChildren
	spanMap[parentSpanID] = parentSpan
}
