// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// CPSpan represents a span used for critical path computation.
// This is a simplified version of ptrace.Span to prevent mutation of the original trace.
type CPSpan struct {
	SpanID       pcommon.SpanID
	ParentSpanID pcommon.SpanID // empty for the root span
	StartTime    uint64         // in microseconds
	Duration     uint64         // in microseconds
	ChildSpanIDs []pcommon.SpanID
}

// CreateCPSpan creates a CPSpan from a ptrace.Span
func CreateCPSpan(span ptrace.Span, childSpanIDs []pcommon.SpanID) CPSpan {
	cpSpan := CPSpan{
		SpanID:       span.SpanID(),
		ParentSpanID: span.ParentSpanID(),
		StartTime:    uint64(span.StartTimestamp()) / 1000, // Convert nanoseconds to microseconds
		Duration:     uint64(span.EndTimestamp()-span.StartTimestamp()) / 1000,
		ChildSpanIDs: make([]pcommon.SpanID, len(childSpanIDs)),
	}
	copy(cpSpan.ChildSpanIDs, childSpanIDs)
	return cpSpan
}

// CreateCPSpanMap creates a map of CPSpan objects from spans with unique span IDs
// (see uniqueSpans). It also builds the parent-child relationships between them.
func CreateCPSpanMap(spans []ptrace.Span) map[pcommon.SpanID]CPSpan {
	spanMap := make(map[pcommon.SpanID]CPSpan, len(spans))
	childrenMap := make(map[pcommon.SpanID][]pcommon.SpanID)

	// First pass: build children map
	for _, span := range spans {
		if !span.ParentSpanID().IsEmpty() {
			parentID := span.ParentSpanID()
			childrenMap[parentID] = append(childrenMap[parentID], span.SpanID())
		}
	}

	// Second pass: create CPSpan objects with child relationships
	for _, span := range spans {
		spanMap[span.SpanID()] = CreateCPSpan(span, childrenMap[span.SpanID()])
	}

	return spanMap
}

// uniqueSpans returns the spans of a trace in order, keeping only the first span
// for each span ID. Every kept span is then the child of at most one span, so no
// cycle can be reached from a root span.
func uniqueSpans(traces ptrace.Traces) []ptrace.Span {
	seen := make(map[pcommon.SpanID]struct{})
	var spans []ptrace.Span
	for _, span := range jptrace.SpanIter(traces) {
		if _, ok := seen[span.SpanID()]; ok {
			continue
		}
		seen[span.SpanID()] = struct{}{}
		spans = append(spans, span)
	}
	return spans
}
