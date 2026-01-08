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
