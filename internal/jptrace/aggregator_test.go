// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
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
