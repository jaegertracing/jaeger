// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// CPSpan represents a span used for critical path computation.
// This is a simplified version of ptrace.Span to prevent mutation of the original trace.
type CPSpan struct {
	SpanID       pcommon.SpanID
	StartTime    uint64 // in microseconds
	Duration     uint64 // in microseconds
	References   []CPSpanReference
	ChildSpanIDs []pcommon.SpanID
}

// CPSpanReference represents a reference between spans
type CPSpanReference struct {
	RefType string // "CHILD_OF" or "FOLLOWS_FROM"
	SpanID  pcommon.SpanID
	TraceID pcommon.TraceID
	Span    *CPSpan // Populated during sanitization
}

// CreateCPSpan creates a CPSpan from a ptrace.Span
func CreateCPSpan(span ptrace.Span, childSpanIDs []pcommon.SpanID) CPSpan {
	cpSpan := CPSpan{
		SpanID:       span.SpanID(),
		StartTime:    uint64(span.StartTimestamp()) / 1000, // Convert nanoseconds to microseconds
		Duration:     uint64(span.EndTimestamp()-span.StartTimestamp()) / 1000,
		ChildSpanIDs: make([]pcommon.SpanID, len(childSpanIDs)),
	}

	// Copy child span IDs
	copy(cpSpan.ChildSpanIDs, childSpanIDs)

	// Convert span links to references
	// In OTLP, parent relationship is implicit via ParentSpanID, not via links
	// We need to handle this differently
	if !span.ParentSpanID().IsEmpty() {
		cpSpan.References = []CPSpanReference{
			{
				RefType: "CHILD_OF",
				SpanID:  span.ParentSpanID(),
				TraceID: span.TraceID(),
			},
		}
	}

	return cpSpan
}

// CreateCPSpanMap creates a map of CPSpan objects from ptrace spans
// It also builds the parent-child relationships by iterating through all spans
func CreateCPSpanMap(traces ptrace.Traces) map[pcommon.SpanID]CPSpan {
	spanMap := make(map[pcommon.SpanID]CPSpan)
	childrenMap := make(map[pcommon.SpanID][]pcommon.SpanID)

	// First pass: build children map
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				if !span.ParentSpanID().IsEmpty() {
					parentID := span.ParentSpanID()
					childrenMap[parentID] = append(childrenMap[parentID], span.SpanID())
				}
			}
		}
	}

	// Second pass: create CPSpan objects with child relationships
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				childSpanIDs := childrenMap[span.SpanID()]
				cpSpan := CreateCPSpan(span, childSpanIDs)
				spanMap[span.SpanID()] = cpSpan
			}
		}
	}

	return spanMap
}
