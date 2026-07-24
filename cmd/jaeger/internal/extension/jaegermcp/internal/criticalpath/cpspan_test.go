// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestCreateCPSpan(t *testing.T) {
	tests := []struct {
		name         string
		span         ptrace.Span
		childSpanIDs []pcommon.SpanID
		expectedSpan CPSpan
	}{
		{
			name: "basic span with no children",
			span: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.SetSpanID(spanID(1))
				span.SetParentSpanID(pcommon.SpanID{})          // no parent
				span.SetStartTimestamp(pcommon.Timestamp(1000)) // 1μs in nanoseconds
				span.SetEndTimestamp(pcommon.Timestamp(101000)) // 101μs
				return span
			}(),
			childSpanIDs: nil,
			expectedSpan: CPSpan{
				SpanID:       spanID(1),
				ParentSpanID: pcommon.SpanID{}, // zero value
				StartTime:    1,                // 1000ns / 1000 = 1μs
				Duration:     100,              // (101000 - 1000) / 1000 = 100μs
				ChildSpanIDs: nil,
			},
		},
		{
			name: "span with a parent reference",
			span: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.SetSpanID(spanID(2))
				span.SetParentSpanID(spanID(1))                  // parent
				span.SetStartTimestamp(pcommon.Timestamp(20000)) // 20μs
				span.SetEndTimestamp(pcommon.Timestamp(40000))   // 40μs
				return span
			}(),
			childSpanIDs: nil,
			expectedSpan: CPSpan{
				SpanID:       spanID(2),
				ParentSpanID: spanID(1),
				StartTime:    20, // 20000ns / 1000 = 20μs
				Duration:     20, // (40000 - 20000) / 1000 = 20μs
				ChildSpanIDs: nil,
			},
		},
		{
			name: "span with no parent reference (zero ParentSpanID)",
			span: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.SetSpanID(spanID(3))
				// ParentSpanID defaults to zero
				span.SetStartTimestamp(pcommon.Timestamp(50000)) // 50μs
				span.SetEndTimestamp(pcommon.Timestamp(60000))   // 60μs
				return span
			}(),
			childSpanIDs: nil,
			expectedSpan: CPSpan{
				SpanID:       spanID(3),
				ParentSpanID: pcommon.SpanID{}, // zero value
				StartTime:    50,               // 50000ns / 1000 = 50μs
				Duration:     10,               // (60000 - 50000) / 1000 = 10μs
				ChildSpanIDs: nil,
			},
		},
		{
			name: "span with multiple child references",
			span: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.SetSpanID(spanID(1))
				span.SetParentSpanID(pcommon.SpanID{})
				span.SetStartTimestamp(pcommon.Timestamp(1000))
				span.SetEndTimestamp(pcommon.Timestamp(101000))
				return span
			}(),
			childSpanIDs: []pcommon.SpanID{
				spanID(2),
				spanID(3),
			},
			expectedSpan: CPSpan{
				SpanID:       spanID(1),
				ParentSpanID: pcommon.SpanID{},
				StartTime:    1,
				Duration:     100,
				ChildSpanIDs: []pcommon.SpanID{
					spanID(2),
					spanID(3),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateCPSpan(tt.span, tt.childSpanIDs)

			assert.Equal(t, tt.expectedSpan.SpanID, result.SpanID, "SpanID should match")
			assert.Equal(t, tt.expectedSpan.ParentSpanID, result.ParentSpanID, "ParentSpanID should match")
			assert.Equal(t, tt.expectedSpan.StartTime, result.StartTime, "StartTime should match")
			assert.Equal(t, tt.expectedSpan.Duration, result.Duration, "Duration should match")
			if tt.expectedSpan.ChildSpanIDs == nil {
				assert.Empty(t, result.ChildSpanIDs, "ChildSpanIDs should be empty")
			} else {
				assert.Equal(t, tt.expectedSpan.ChildSpanIDs, result.ChildSpanIDs, "ChildSpanIDs should match")
			}
		})
	}
}

func TestCreateCPSpanMap(t *testing.T) {
	tests := []struct {
		name          string
		traces        ptrace.Traces
		expectedCount int
		verifyFunc    func(t *testing.T, spanMap map[pcommon.SpanID]CPSpan)
	}{
		{
			name:          "empty trace (no spans)",
			traces:        ptrace.NewTraces(),
			expectedCount: 0,
			verifyFunc: func(t *testing.T, spanMap map[pcommon.SpanID]CPSpan) {
				assert.Empty(t, spanMap, "map should be empty")
			},
		},
		{
			name: "single span with no parent",
			traces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()
				span := ss.Spans().AppendEmpty()
				span.SetSpanID(spanID(1))
				span.SetParentSpanID(pcommon.SpanID{}) // no parent
				span.SetStartTimestamp(pcommon.Timestamp(1000))
				span.SetEndTimestamp(pcommon.Timestamp(101000))
				return traces
			}(),
			expectedCount: 1,
			verifyFunc: func(t *testing.T, spanMap map[pcommon.SpanID]CPSpan) {
				assert.Len(t, spanMap, 1, "map should have one entry")
				span := spanMap[spanID(1)]
				assert.Equal(t, pcommon.SpanID{}, span.ParentSpanID, "span should have no parent")
				assert.Empty(t, span.ChildSpanIDs, "span should have no children")
			},
		},
		{
			name: "two spans, parent-child relationship",
			traces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()

				// Parent span
				parent := ss.Spans().AppendEmpty()
				parent.SetSpanID(spanID(1))
				parent.SetParentSpanID(pcommon.SpanID{})
				parent.SetStartTimestamp(pcommon.Timestamp(1000))
				parent.SetEndTimestamp(pcommon.Timestamp(101000))

				// Child span
				child := ss.Spans().AppendEmpty()
				child.SetSpanID(spanID(2))
				child.SetParentSpanID(spanID(1))
				child.SetStartTimestamp(pcommon.Timestamp(20000))
				child.SetEndTimestamp(pcommon.Timestamp(40000))

				return traces
			}(),
			expectedCount: 2,
			verifyFunc: func(t *testing.T, spanMap map[pcommon.SpanID]CPSpan) {
				assert.Len(t, spanMap, 2, "map should have two entries")

				// Verify parent has child in ChildSpanIDs
				parent := spanMap[spanID(1)]
				assert.Equal(t, pcommon.SpanID{}, parent.ParentSpanID, "parent should have no parent")
				assert.Len(t, parent.ChildSpanIDs, 1, "parent should have one child")
				assert.Equal(t, spanID(2), parent.ChildSpanIDs[0], "parent's child should be span 2")

				// Verify child has correct ParentSpanID
				child := spanMap[spanID(2)]
				assert.Equal(t, spanID(1), child.ParentSpanID, "child should have parent span 1")
				assert.Empty(t, child.ChildSpanIDs, "child should have no children")
			},
		},
		{
			name: "three spans, multiple children of same parent",
			traces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()

				// Parent span
				parent := ss.Spans().AppendEmpty()
				parent.SetSpanID(spanID(1))
				parent.SetParentSpanID(pcommon.SpanID{})
				parent.SetStartTimestamp(pcommon.Timestamp(1000))
				parent.SetEndTimestamp(pcommon.Timestamp(101000))

				// Child 1
				child1 := ss.Spans().AppendEmpty()
				child1.SetSpanID(spanID(2))
				child1.SetParentSpanID(spanID(1))
				child1.SetStartTimestamp(pcommon.Timestamp(20000))
				child1.SetEndTimestamp(pcommon.Timestamp(40000))

				// Child 2
				child2 := ss.Spans().AppendEmpty()
				child2.SetSpanID(spanID(3))
				child2.SetParentSpanID(spanID(1))
				child2.SetStartTimestamp(pcommon.Timestamp(50000))
				child2.SetEndTimestamp(pcommon.Timestamp(60000))

				return traces
			}(),
			expectedCount: 3,
			verifyFunc: func(t *testing.T, spanMap map[pcommon.SpanID]CPSpan) {
				assert.Len(t, spanMap, 3, "map should have three entries")

				// Verify parent has both children in ChildSpanIDs
				parent := spanMap[spanID(1)]
				assert.Len(t, parent.ChildSpanIDs, 2, "parent should have two children")

				// Check that both children are present (order may vary)
				childIDs := make(map[pcommon.SpanID]bool)
				for _, id := range parent.ChildSpanIDs {
					childIDs[id] = true
				}
				assert.True(t, childIDs[spanID(2)], "child 2 should be in parent's children")
				assert.True(t, childIDs[spanID(3)], "child 3 should be in parent's children")

				// Verify children have correct ParentSpanID
				child1 := spanMap[spanID(2)]
				assert.Equal(t, spanID(1), child1.ParentSpanID)

				child2 := spanMap[spanID(3)]
				assert.Equal(t, spanID(1), child2.ParentSpanID)
			},
		},
		{
			name: "span referencing a non-existent parent",
			traces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()

				// Span with a parent that doesn't exist in the trace
				span := ss.Spans().AppendEmpty()
				span.SetSpanID(spanID(1))
				span.SetParentSpanID(spanID(99)) // parent not in trace
				span.SetStartTimestamp(pcommon.Timestamp(1000))
				span.SetEndTimestamp(pcommon.Timestamp(101000))

				return traces
			}(),
			expectedCount: 1,
			verifyFunc: func(t *testing.T, spanMap map[pcommon.SpanID]CPSpan) {
				assert.Len(t, spanMap, 1, "map should have one entry")
				span := spanMap[spanID(1)]
				assert.Equal(t, spanID(99), span.ParentSpanID, "span should reference non-existent parent")
				assert.Empty(t, span.ChildSpanIDs, "span should have no children")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateCPSpanMap(tt.traces)

			assert.Len(t, result, tt.expectedCount, "map should have expected number of entries")
			if tt.verifyFunc != nil {
				tt.verifyFunc(t, result)
			}
		})
	}
}
