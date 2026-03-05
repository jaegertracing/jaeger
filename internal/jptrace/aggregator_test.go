// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestAggregateTraces_AggregatesSpansWithSameTraceID(t *testing.T) {
	trace1 := ptrace.NewTraces()
	resource1 := trace1.ResourceSpans().AppendEmpty()
	scope1 := resource1.ScopeSpans().AppendEmpty()
	span1 := scope1.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID([16]byte{1}))
	span1.SetName("span1")

	trace1Continued := ptrace.NewTraces()
	resource2 := trace1Continued.ResourceSpans().AppendEmpty()
	scope2 := resource2.ScopeSpans().AppendEmpty()
	span2 := scope2.Spans().AppendEmpty()
	span2.SetTraceID(pcommon.TraceID([16]byte{1}))
	span2.SetName("span2")

	trace2 := ptrace.NewTraces()
	resource3 := trace2.ResourceSpans().AppendEmpty()
	scope3 := resource3.ScopeSpans().AppendEmpty()
	span3 := scope3.Spans().AppendEmpty()
	span3.SetTraceID(pcommon.TraceID([16]byte{2}))
	span3.SetName("span3")

	trace3 := ptrace.NewTraces()
	resource4 := trace3.ResourceSpans().AppendEmpty()
	scope4 := resource4.ScopeSpans().AppendEmpty()
	span4 := scope4.Spans().AppendEmpty()
	span4.SetTraceID(pcommon.TraceID([16]byte{3}))
	span4.SetName("span4")

	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{trace1, trace1Continued, trace2}, nil)
		yield([]ptrace.Traces{trace3}, nil)
	}

	var result []ptrace.Traces
	AggregateTraces(tracesSeq)(func(trace ptrace.Traces, _ error) bool {
		result = append(result, trace)
		return true
	})

	require.Len(t, result, 3)

	require.Equal(t, 2, result[0].ResourceSpans().Len())
	require.Equal(t, 1, result[1].ResourceSpans().Len())
	require.Equal(t, 1, result[2].ResourceSpans().Len())

	gotSpan1 := result[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, gotSpan1.TraceID(), pcommon.TraceID([16]byte{1}))
	require.Equal(t, "span1", gotSpan1.Name())

	gotSpan2 := result[0].ResourceSpans().At(1).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, gotSpan2.TraceID(), pcommon.TraceID([16]byte{1}))
	require.Equal(t, "span2", gotSpan2.Name())

	gotSpan3 := result[1].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, gotSpan3.TraceID(), pcommon.TraceID([16]byte{2}))
	require.Equal(t, "span3", gotSpan3.Name())

	gotSpan4 := result[2].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, gotSpan4.TraceID(), pcommon.TraceID([16]byte{3}))
	require.Equal(t, "span4", gotSpan4.Name())
}

func TestAggregateTraces_YieldsErrorFromTracesSeq(t *testing.T) {
	trace1 := ptrace.NewTraces()
	resource1 := trace1.ResourceSpans().AppendEmpty()
	scope1 := resource1.ScopeSpans().AppendEmpty()
	span1 := scope1.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID([16]byte{1}))
	span1.SetName("span1")

	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		if !yield(nil, assert.AnError) {
			return
		}
		yield([]ptrace.Traces{trace1}, nil) // should not get here
	}
	aggregatedSeq := AggregateTraces(tracesSeq)

	var lastResult ptrace.Traces
	var lastErr error
	aggregatedSeq(func(trace ptrace.Traces, e error) bool {
		lastResult = trace
		if e != nil {
			lastErr = e
		}
		return true
	})

	require.ErrorIs(t, lastErr, assert.AnError)
	require.Equal(t, ptrace.NewTraces(), lastResult)
}

func TestAggregateTraces_RespectsEarlyReturn(t *testing.T) {
	trace1 := ptrace.NewTraces()
	resource1 := trace1.ResourceSpans().AppendEmpty()
	scope1 := resource1.ScopeSpans().AppendEmpty()
	span1 := scope1.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID([16]byte{1}))
	span1.SetName("span1")

	trace2 := ptrace.NewTraces()
	resource2 := trace2.ResourceSpans().AppendEmpty()
	scope2 := resource2.ScopeSpans().AppendEmpty()
	span2 := scope2.Spans().AppendEmpty()
	span2.SetTraceID(pcommon.TraceID([16]byte{2}))
	span2.SetName("span2")

	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{trace1}, nil)
		yield([]ptrace.Traces{trace2}, nil)
	}
	aggregatedSeq := AggregateTraces(tracesSeq)

	var lastResult ptrace.Traces
	aggregatedSeq(func(trace ptrace.Traces, _ error) bool {
		lastResult = trace
		return false
	})

	require.Equal(t, trace1, lastResult)
}

func TestAggregateTracesWithLimit(t *testing.T) {
	createTrace := func(traceID byte, spanCount int) ptrace.Traces {
		trace := ptrace.NewTraces()
		spans := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
		for i := 0; i < spanCount; i++ {
			span := spans.AppendEmpty()
			span.SetTraceID(pcommon.TraceID([16]byte{traceID}))
		}
		return trace
	}

	tests := []struct {
		name           string
		maxSize        int
		inputSpans     int
		expectedSpans  int
		expectTruncate bool
	}{
		{"no_limit", 0, 5, 5, false},
		{"under_limit", 10, 5, 5, false},
		{"over_limit", 3, 5, 3, true},
		{"exact_limit", 5, 5, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{createTrace(1, tt.inputSpans)}, nil)
			}

			var result []ptrace.Traces
			AggregateTracesWithLimit(tracesSeq, tt.maxSize)(func(trace ptrace.Traces, _ error) bool {
				result = append(result, trace)
				return true
			})

			require.Len(t, result, 1)
			assert.Equal(t, tt.expectedSpans, result[0].SpanCount())

			// Check for truncation warning
			if tt.expectTruncate {
				firstSpan := result[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
				warnings := GetWarnings(firstSpan)
				assert.NotEmpty(t, warnings, "expected truncation warning")
				assert.Contains(t, warnings[len(warnings)-1], fmt.Sprintf("trace has more than %d spans", tt.maxSize))
			}
		})
	}
}

func TestCopySpansUpToLimit(t *testing.T) {
	src := ptrace.NewTraces()
	spans := src.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for i := 0; i < 5; i++ {
		spans.AppendEmpty().SetName("span")
	}

	dest := ptrace.NewTraces()
	copySpansUpToLimit(dest, src, 3)

	assert.Equal(t, 3, dest.SpanCount())
}

func TestCopySpansUpToLimit_MultipleResourceSpans(t *testing.T) {
	src := ptrace.NewTraces()
	rs0 := src.ResourceSpans().AppendEmpty()
	ss0 := rs0.ScopeSpans().AppendEmpty()
	ss0.Spans().AppendEmpty().SetName("rs0-span0")
	ss0.Spans().AppendEmpty().SetName("rs0-span1")
	rs1 := src.ResourceSpans().AppendEmpty()
	ss1 := rs1.ScopeSpans().AppendEmpty()
	ss1.Spans().AppendEmpty().SetName("rs1-span0")
	ss1.Spans().AppendEmpty().SetName("rs1-span1")

	dest := ptrace.NewTraces()
	copySpansUpToLimit(dest, src, 3)

	require.Equal(t, 3, dest.SpanCount())
	require.Equal(t, 2, dest.ResourceSpans().Len())
	assert.Equal(t, 2, dest.ResourceSpans().At(0).ScopeSpans().At(0).Spans().Len())
	assert.Equal(t, 1, dest.ResourceSpans().At(1).ScopeSpans().At(0).Spans().Len())
}

func TestCopySpansUpToLimit_MultipleScopeSpans(t *testing.T) {
	src := ptrace.NewTraces()
	rs := src.ResourceSpans().AppendEmpty()
	ss0 := rs.ScopeSpans().AppendEmpty()
	ss0.Spans().AppendEmpty().SetName("ss0-span0")
	ss0.Spans().AppendEmpty().SetName("ss0-span1")
	ss1 := rs.ScopeSpans().AppendEmpty()
	ss1.Spans().AppendEmpty().SetName("ss1-span0")
	ss1.Spans().AppendEmpty().SetName("ss1-span1")

	dest := ptrace.NewTraces()
	copySpansUpToLimit(dest, src, 3)

	require.Equal(t, 3, dest.SpanCount())
	require.Equal(t, 1, dest.ResourceSpans().Len())
	destScopes := dest.ResourceSpans().At(0).ScopeSpans()
	require.Equal(t, 2, destScopes.Len())
	assert.Equal(t, 2, destScopes.At(0).Spans().Len())
	assert.Equal(t, 1, destScopes.At(1).Spans().Len())
}

func TestCopySpansUpToLimit_NoEmptyContainers(t *testing.T) {
	// src has two resources: the first has no scopes, the second has spans.
	// copySpansUpToLimit should not create an empty ResourceSpans for the first resource.
	src := ptrace.NewTraces()
	src.ResourceSpans().AppendEmpty() // empty resource, no scopes
	spans := src.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for i := 0; i < 3; i++ {
		spans.AppendEmpty().SetName("span")
	}

	dest := ptrace.NewTraces()
	copySpansUpToLimit(dest, src, 2)

	assert.Equal(t, 2, dest.SpanCount())
	assert.Equal(t, 1, dest.ResourceSpans().Len(), "empty resource should not be copied")
}

func TestAggregateTracesWithLimit_MultiBatch(t *testing.T) {
	// A trace that arrives in three batches should produce exactly one truncation
	// warning even when subsequent batches arrive after the limit is already reached.
	createBatch := func(traceID byte, spanCount int) ptrace.Traces {
		trace := ptrace.NewTraces()
		spans := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
		for i := 0; i < spanCount; i++ {
			span := spans.AppendEmpty()
			span.SetTraceID(pcommon.TraceID([16]byte{traceID}))
		}
		return trace
	}

	// Limit is 3. Batch 1: 2 spans (under limit). Batch 2: 2 spans (partial copy, hits limit).
	// Batch 3: 2 spans (already at limit, ignored).
	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		if !yield([]ptrace.Traces{createBatch(1, 2)}, nil) {
			return
		}
		if !yield([]ptrace.Traces{createBatch(1, 2)}, nil) {
			return
		}
		yield([]ptrace.Traces{createBatch(1, 2)}, nil)
	}

	var result []ptrace.Traces
	AggregateTracesWithLimit(tracesSeq, 3)(func(trace ptrace.Traces, _ error) bool {
		result = append(result, trace)
		return true
	})

	require.Len(t, result, 1)
	assert.Equal(t, 3, result[0].SpanCount())

	firstSpan := result[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	warnings := GetWarnings(firstSpan)
	assert.Len(t, warnings, 1, "should have exactly one truncation warning, not one per extra batch")
	assert.Contains(t, warnings[0], fmt.Sprintf("trace has more than %d spans", 3))
}

// TestAggregateTracesWithLimit_ExactLimitThenOverflow specifically tests the scenario
// where the first batch fills the trace to exactly maxSize (no warning yet), and a
// subsequent batch then causes the first overflow and must trigger the truncation warning.
func TestAggregateTracesWithLimit_ExactLimitThenOverflow(t *testing.T) {
	createBatch := func(traceID byte, spanCount int) ptrace.Traces {
		trace := ptrace.NewTraces()
		spans := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
		for i := 0; i < spanCount; i++ {
			span := spans.AppendEmpty()
			span.SetTraceID(pcommon.TraceID([16]byte{traceID}))
		}
		return trace
	}

	// Batch 1 has exactly maxSize spans — fits without truncation, no warning added yet.
	// Batch 2 has 1 more span — must be dropped AND must trigger the warning.
	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		if !yield([]ptrace.Traces{createBatch(1, 3)}, nil) {
			return
		}
		yield([]ptrace.Traces{createBatch(1, 1)}, nil)
	}

	var result []ptrace.Traces
	AggregateTracesWithLimit(tracesSeq, 3)(func(trace ptrace.Traces, _ error) bool {
		result = append(result, trace)
		return true
	})

	require.Len(t, result, 1)
	assert.Equal(t, 3, result[0].SpanCount())

	firstSpan := result[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	warnings := GetWarnings(firstSpan)
	assert.Len(t, warnings, 1, "overflow after exact-limit batch must produce exactly one truncation warning")
	assert.Contains(t, warnings[0], fmt.Sprintf("trace has more than %d spans", 3))
}

func TestMarkAndCheckTruncated(t *testing.T) {
	trace := ptrace.NewTraces()
	firstSpan := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	assert.Empty(t, GetWarnings(firstSpan))
	markTraceTruncated(trace, 10)
	// Now should have truncation warning
	warnings := GetWarnings(firstSpan)
	assert.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "trace has more than 10 spans")
}

func TestAggregateTraces_HandlesEmptyTraces(t *testing.T) {
	emptyTrace := ptrace.NewTraces() // No resource spans

	traceWithNoSpans := ptrace.NewTraces()
	traceWithNoSpans.ResourceSpans().AppendEmpty() // Has resource spans but no scope spans

	traceWithNoSpans2 := ptrace.NewTraces()
	rs := traceWithNoSpans2.ResourceSpans().AppendEmpty()
	rs.ScopeSpans().AppendEmpty() // Has scope spans but no spans

	trace1 := ptrace.NewTraces()
	rs1 := trace1.ResourceSpans().AppendEmpty()
	ss1 := rs1.ScopeSpans().AppendEmpty()
	span1 := ss1.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID([16]byte{1}))

	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{emptyTrace, traceWithNoSpans, traceWithNoSpans2, trace1}, nil)
	}

	var result []ptrace.Traces
	AggregateTraces(tracesSeq)(func(trace ptrace.Traces, _ error) bool {
		result = append(result, trace)
		return true
	})

	require.Len(t, result, 1)
	require.Equal(t, trace1, result[0])
}

func TestAggregateTraces_DoesNotYieldAfterConsumerStops(t *testing.T) {
	// This test demonstrates why the `cont` variable is needed in AggregateTraces.
	// Without it, the function would violate the iterator protocol by calling yield
	// after the consumer has returned false.
	//
	// Setup: Create two separate traces with different IDs that will be yielded
	// from separate batches. The consumer will stop after the first trace.
	trace1 := ptrace.NewTraces()
	resource1 := trace1.ResourceSpans().AppendEmpty()
	scope1 := resource1.ScopeSpans().AppendEmpty()
	span1 := scope1.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID([16]byte{1}))
	span1.SetName("span1")

	trace2 := ptrace.NewTraces()
	resource2 := trace2.ResourceSpans().AppendEmpty()
	scope2 := resource2.ScopeSpans().AppendEmpty()
	span2 := scope2.Spans().AppendEmpty()
	span2.SetTraceID(pcommon.TraceID([16]byte{2}))
	span2.SetName("span2")

	// Yield traces in separate batches - this ensures the final yield happens
	// after the iterator completes, which is where the bug would manifest.
	tracesSeq := func(yield func([]ptrace.Traces, error) bool) {
		if !yield([]ptrace.Traces{trace1}, nil) {
			return
		}
		yield([]ptrace.Traces{trace2}, nil)
	}

	var yieldCount int
	aggregatedSeq := AggregateTraces(tracesSeq)

	// Consumer stops after first yield
	aggregatedSeq(func(_ ptrace.Traces, _ error) bool {
		yieldCount++
		return false // Stop iteration after first trace
	})

	// Without the `cont` variable, this would panic with:
	// "runtime error: range function continued iteration after function for loop body returned false"
	// The cont variable prevents the final yield (line 48-50 in aggregator.go) from
	// being called after the consumer has already returned false.
	require.Equal(t, 1, yieldCount, "yield should only be called once since consumer returned false")
}
