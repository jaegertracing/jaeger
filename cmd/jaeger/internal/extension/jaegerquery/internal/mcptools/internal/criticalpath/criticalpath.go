// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Section represents a section of the critical path
type Section struct {
	SpanID       string `json:"span_id"`
	SectionStart uint64 `json:"section_start"` // in microseconds
	SectionEnd   uint64 `json:"section_end"`   // in microseconds
}

// computeCriticalPath computes the critical path sections of a trace.
// The algorithm begins with the top-level span and iterates through the last finishing children (LFCs).
// It computes the critical path for each LFC span in turn.
// When a span has no LFC left, the algorithm walks backward to its parent and picks another
// child that finished just before that span's start.
//
// Parameters:
//   - spanMap: A map associating span IDs with spans
//   - spanID: The ID of the current span
//   - criticalPath: An array of critical path sections (accumulated result)
//   - returningChildStartTime: Optional parameter representing the span's start time.
//     It is provided only during the recursive return phase.
//
// Returns: An array of critical path sections for the trace
//
// Example:
//
//	|-------------spanA--------------|
//	   |--spanB--|    |--spanC--|
//
// The LFC of spanA is spanC, as it finishes last among its child spans.
// After invoking CP recursively on LFC, for spanC there is no LFC, so the algorithm walks backward.
// At this point, it uses returningChildStartTime (startTime of spanC) to select another child that finished
// immediately before the LFC's start.
func computeCriticalPath(
	spanMap map[pcommon.SpanID]CPSpan,
	spanID pcommon.SpanID,
	criticalPath []Section,
	returningChildStartTime *uint64,
) []Section {
	// In a tree, each span is entered once from its parent and once more each time
	// one of its children returns to it, so the walk takes fewer than
	// 2*len(spanMap) steps. The bound stops the walk if spanMap has a cycle.
	for range 2 * len(spanMap) {
		currentSpan, ok := spanMap[spanID]
		if !ok {
			return criticalPath
		}

		lastFinishingChildSpan := findLastFinishingChildSpan(spanMap, currentSpan, returningChildStartTime)

		var spanCriticalSection Section

		if lastFinishingChildSpan != nil {
			// There is a last finishing child
			endTime := currentSpan.StartTime + currentSpan.Duration
			if returningChildStartTime != nil {
				endTime = *returningChildStartTime
			}

			spanCriticalSection = Section{
				SpanID:       currentSpan.SpanID.String(),
				SectionStart: lastFinishingChildSpan.StartTime + lastFinishingChildSpan.Duration,
				SectionEnd:   endTime,
			}

			if spanCriticalSection.SectionStart != spanCriticalSection.SectionEnd {
				criticalPath = append(criticalPath, spanCriticalSection)
			}

			// Now focus shifts to the lastFinishingChildSpan of current span
			spanID = lastFinishingChildSpan.SpanID
			returningChildStartTime = nil
		} else {
			// If there is no last finishing child then total section up to startTime of span is on critical path
			endTime := currentSpan.StartTime + currentSpan.Duration
			if returningChildStartTime != nil {
				endTime = *returningChildStartTime
			}

			spanCriticalSection = Section{
				SpanID:       currentSpan.SpanID.String(),
				SectionStart: currentSpan.StartTime,
				SectionEnd:   endTime,
			}

			if spanCriticalSection.SectionStart != spanCriticalSection.SectionEnd {
				criticalPath = append(criticalPath, spanCriticalSection)
			}

			// Now as there are no LFCs focus shifts to parent span from startTime of span
			// walk backwards to one level depth to parent span
			// provide span's startTime as returningChildStartTime
			if currentSpan.ParentSpanID.IsEmpty() {
				return criticalPath
			}
			spanID = currentSpan.ParentSpanID
			returningChildStartTime = &currentSpan.StartTime
		}
	}

	return criticalPath
}

// ComputeCriticalPathFromTraces computes the critical path for a given trace
func ComputeCriticalPathFromTraces(traces ptrace.Traces) ([]Section, error) {
	spans := uniqueSpans(traces)

	// Find the root span (the one with no parent)
	var rootSpanID pcommon.SpanID
	found := false

	for _, span := range spans {
		if span.ParentSpanID().IsEmpty() {
			rootSpanID = span.SpanID()
			found = true
			break
		}
	}

	if !found {
		return nil, errors.New("no root span found in trace")
	}

	// Create a map of CPSpan objects to avoid modifying the original trace
	spanMap := CreateCPSpanMap(spans)
	if len(spanMap) == 0 {
		return nil, errors.New("empty trace")
	}

	sanitizedSpanMap := removeOverflowingChildren(spanMap)
	// An empty critical path is a valid result (e.g. a single zero-duration root
	// span), so start from a non-nil slice and never treat emptiness as an error.
	criticalPath := computeCriticalPath(sanitizedSpanMap, rootSpanID, []Section{}, nil)

	return criticalPath, nil
}
