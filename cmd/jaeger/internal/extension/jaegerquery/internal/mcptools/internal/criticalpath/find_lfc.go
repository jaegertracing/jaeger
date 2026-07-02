// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// findLastFinishingChildSpan returns the span that finished last among the child spans.
// If returningChildStartTime is provided, it returns the child span that finishes
// just before the specified returningChildStartTime.
func findLastFinishingChildSpan(
	spanMap map[pcommon.SpanID]CPSpan,
	currentSpan CPSpan,
	returningChildStartTime *uint64,
) *CPSpan {
	var lastFinishingChildSpan *CPSpan
	var maxEndTime uint64 = 0

	for _, childID := range currentSpan.ChildSpanIDs {
		childSpan, ok := spanMap[childID]
		if !ok {
			continue
		}

		childEndTime := childSpan.StartTime + childSpan.Duration

		if returningChildStartTime != nil {
			// Find child that finishes just before the returning child's start time
			if childEndTime < *returningChildStartTime {
				if childEndTime > maxEndTime {
					maxEndTime = childEndTime
					childSpanCopy := childSpan
					lastFinishingChildSpan = &childSpanCopy
				}
			}
		} else {
			// Find the child that finishes last
			if childEndTime > maxEndTime {
				maxEndTime = childEndTime
				childSpanCopy := childSpan
				lastFinishingChildSpan = &childSpanCopy
			}
		}
	}

	return lastFinishingChildSpan
}
