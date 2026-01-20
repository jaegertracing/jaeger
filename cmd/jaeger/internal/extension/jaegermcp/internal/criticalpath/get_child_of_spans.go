// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// getChildOfSpans removes spans whose refType is FOLLOWS_FROM and their descendants.
// Returns a map with only CHILD_OF relationship spans.
func getChildOfSpans(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	var followFromSpanIDs []pcommon.SpanID
	var followFromSpansDescendantIDs []pcommon.SpanID

	// First find all FOLLOWS_FROM refType spans
	for spanID, span := range spanMap {
		if len(span.References) > 0 && span.References[0].RefType == "FOLLOWS_FROM" {
			followFromSpanIDs = append(followFromSpanIDs, spanID)
			// Remove the spanID from childSpanIDs array of its parent span
			if parentSpan, ok := spanMap[span.References[0].SpanID]; ok {
				filteredChildren := make([]pcommon.SpanID, 0, len(parentSpan.ChildSpanIDs))
				for _, childID := range parentSpan.ChildSpanIDs {
					if childID != spanID {
						filteredChildren = append(filteredChildren, childID)
					}
				}
				parentSpan.ChildSpanIDs = filteredChildren
				spanMap[span.References[0].SpanID] = parentSpan
			}
		}
	}

	// Recursively find all descendants of FOLLOWS_FROM spans
	var findDescendantSpans func(spanIDs []pcommon.SpanID)
	findDescendantSpans = func(spanIDs []pcommon.SpanID) {
		for _, spanID := range spanIDs {
			if span, ok := spanMap[spanID]; ok {
				if len(span.ChildSpanIDs) > 0 {
					followFromSpansDescendantIDs = append(followFromSpansDescendantIDs, span.ChildSpanIDs...)
					findDescendantSpans(span.ChildSpanIDs)
				}
			}
		}
	}
	findDescendantSpans(followFromSpanIDs)

	// Delete all FOLLOWS_FROM spans and their descendants
	idsToBeDeleted := append(followFromSpanIDs, followFromSpansDescendantIDs...)
	for _, id := range idsToBeDeleted {
		delete(spanMap, id)
	}

	return spanMap
}
