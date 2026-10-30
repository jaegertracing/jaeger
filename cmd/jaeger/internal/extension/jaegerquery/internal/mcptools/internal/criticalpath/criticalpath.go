// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"errors"
	"strconv"

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
// It recursively computes the critical path for each LFC span.
// Upon return from recursion, the algorithm walks backward and picks another child that
// finished just before the LFC's start.
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
	return computeCriticalPathWithGuard(spanMap, spanID, criticalPath, returningChildStartTime, make(map[string]struct{}))
}

// criticalPathVisitKey returns a unique key for a (spanID, returningChildStartTime)
// recursion invocation. It is used to detect cyclic or self-referencing span
// relationships that would otherwise cause computeCriticalPath to recurse
// indefinitely (see issue #9966).
func criticalPathVisitKey(spanID pcommon.SpanID, returningChildStartTime *uint64) string {
	if returningChildStartTime == nil {
		return spanID.String() + "|-"
	}
	return spanID.String() + "|" + strconv.FormatUint(*returningChildStartTime, 10)
}

// computeCriticalPathWithGuard is the internal implementation of
// computeCriticalPath that tracks the current recursion path. When a span is
// revisited with the same parameters, a cycle exists in the span
// parent/child graph (e.g. Root -> A -> B -> A, or a self-referencing span
// whose ParentSpanID equals its SpanID). In that case the recursion stops to
// avoid an infinite loop / stack overflow while still returning the sections
// accumulated so far.
func computeCriticalPathWithGuard(
	spanMap map[pcommon.SpanID]CPSpan,
	spanID pcommon.SpanID,
	criticalPath []Section,
	returningChildStartTime *uint64,
	visitGuard map[string]struct{},
) []Section {
	currentSpan, ok := spanMap[spanID]
	if !ok {
		return criticalPath
	}

	visitKey := criticalPathVisitKey(spanID, returningChildStartTime)
	if _, visited := visitGuard[visitKey]; visited {
		// A cycle or self-reference was detected. Stop descending further to
		// avoid infinite recursion; the sections computed so far are valid.
		return criticalPath
	}
	visitGuard[visitKey] = struct{}{}
	defer delete(visitGuard, visitKey)

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
		criticalPath = computeCriticalPathWithGuard(spanMap, lastFinishingChildSpan.SpanID, criticalPath, nil, visitGuard)
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
		// return from recursion and walk backwards to one level depth to parent span
		// provide span's startTime as returningChildStartTime
		if !currentSpan.ParentSpanID.IsEmpty() {
			criticalPath = computeCriticalPathWithGuard(spanMap, currentSpan.ParentSpanID, criticalPath, &currentSpan.StartTime, visitGuard)
		}
	}

	return criticalPath
}

// ComputeCriticalPathFromTraces computes the critical path for a given trace
func ComputeCriticalPathFromTraces(traces ptrace.Traces) ([]Section, error) {
	// Find the root span (the one with no parent)
	var rootSpanID pcommon.SpanID
	found := false

	for i := 0; i < traces.ResourceSpans().Len() && !found; i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len() && !found; j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				if span.ParentSpanID().IsEmpty() {
					rootSpanID = span.SpanID()
					found = true
					break
				}
			}
		}
	}

	if !found {
		return nil, errors.New("no root span found in trace")
	}

	// Create a map of CPSpan objects to avoid modifying the original trace
	spanMap := CreateCPSpanMap(traces)
	if len(spanMap) == 0 {
		return nil, errors.New("empty trace")
	}

	sanitizedSpanMap := removeOverflowingChildren(spanMap)
	// An empty critical path is a valid result (e.g. a single zero-duration root
	// span), so start from a non-nil slice and never treat emptiness as an error.
	criticalPath := computeCriticalPath(sanitizedSpanMap, rootSpanID, []Section{}, nil)

	return criticalPath, nil
}
