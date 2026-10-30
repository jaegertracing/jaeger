// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"cmp"
	"slices"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

type orderedChild struct {
	span     CPSpan
	position int
}

type childSpanIterator []orderedChild

func newChildSpanIterator(spanMap map[pcommon.SpanID]CPSpan, parent CPSpan) childSpanIterator {
	children := make(childSpanIterator, 0, len(parent.ChildSpanIDs))
	for position, id := range parent.ChildSpanIDs {
		if child, ok := spanMap[id]; ok {
			children = append(children, orderedChild{span: child, position: position})
		}
	}
	slices.SortFunc(children, func(a, b orderedChild) int {
		if order := cmp.Compare(b.span.StartTime+b.span.Duration, a.span.StartTime+a.span.Duration); order != 0 {
			return order
		}
		// The original scan chooses the first child when finish times are equal.
		return cmp.Compare(a.position, b.position)
	})
	return children
}

// next returns the last finishing child before the returning child's start.
// The boundary only moves backward during the walk, so skipped children cannot
// become eligible later. Each child is inspected at most once after sorting.
func (children *childSpanIterator) next(returningChildStartTime *uint64) *CPSpan {
	for len(*children) > 0 {
		child := &(*children)[0].span
		*children = (*children)[1:]
		if childFinishesBefore(*child, returningChildStartTime) {
			return child
		}
	}
	return nil
}

func childFinishesBefore(child CPSpan, boundary *uint64) bool {
	endTime := child.StartTime + child.Duration
	return endTime > 0 && (boundary == nil || (child.StartTime < *boundary && endTime <= *boundary))
}
