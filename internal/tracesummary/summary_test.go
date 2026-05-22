// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracesummary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var testTraceID = pcommon.TraceID([16]byte{1})

func TestFromTrace(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	trace := ptrace.NewTraces()
	appendSpan(trace, "frontend", "root", [8]byte{1}, [8]byte{}, start, 20*time.Millisecond, ptrace.StatusCodeUnset)
	appendSpan(trace, "backend", "child", [8]byte{2}, [8]byte{1}, start.Add(time.Millisecond), 5*time.Millisecond, ptrace.StatusCodeError)
	appendSpan(trace, "frontend", "child-2", [8]byte{3}, [8]byte{1}, start.Add(2*time.Millisecond), 2*time.Millisecond, ptrace.StatusCodeError)

	summary := FromTrace(trace)

	require.Equal(t, testTraceID, summary.TraceID)
	assert.Equal(t, "frontend", summary.RootServiceName)
	assert.Equal(t, "root", summary.RootOperationName)
	assert.Equal(t, start, summary.MinStartTime)
	assert.Equal(t, start.Add(20*time.Millisecond), summary.MaxEndTime)
	assert.Equal(t, 3, summary.SpanCount)
	assert.Equal(t, 2, summary.ErrorSpanCount)
	assert.Zero(t, summary.OrphanSpanCount)
	assert.Equal(t, "backend", summary.Services[0].Name)
	assert.Equal(t, 1, summary.Services[0].SpanCount)
	assert.Equal(t, 1, summary.Services[0].ErrorSpanCount)
	assert.Equal(t, "frontend", summary.Services[1].Name)
	assert.Equal(t, 2, summary.Services[1].SpanCount)
	assert.Equal(t, 1, summary.Services[1].ErrorSpanCount)
}

func TestFromTraceSelectsDeterministicRoot(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	trace := ptrace.NewTraces()
	appendSpan(trace, "svc-b", "root-b", [8]byte{2}, [8]byte{}, start, time.Millisecond, ptrace.StatusCodeUnset)
	appendSpan(trace, "svc-a", "root-a", [8]byte{1}, [8]byte{}, start, time.Millisecond, ptrace.StatusCodeUnset)

	summary := FromTrace(trace)

	assert.Equal(t, "svc-a", summary.RootServiceName)
	assert.Equal(t, "root-a", summary.RootOperationName)
}

func TestFromTraceUsesOrphanAsRootCandidate(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	trace := ptrace.NewTraces()
	appendSpan(trace, "svc", "orphan", [8]byte{1}, [8]byte{9}, start, time.Millisecond, ptrace.StatusCodeUnset)

	summary := FromTrace(trace)

	assert.Equal(t, "svc", summary.RootServiceName)
	assert.Equal(t, "orphan", summary.RootOperationName)
	assert.Equal(t, 1, summary.OrphanSpanCount)
}

func TestFromTraceFallsBackWhenTraceHasNoRoot(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	trace := ptrace.NewTraces()
	appendSpan(trace, "svc-b", "cycle-b", [8]byte{2}, [8]byte{1}, start.Add(time.Millisecond), time.Millisecond, ptrace.StatusCodeUnset)
	appendSpan(trace, "svc-a", "cycle-a", [8]byte{1}, [8]byte{2}, start, time.Millisecond, ptrace.StatusCodeUnset)

	summary := FromTrace(trace)

	assert.Equal(t, "svc-a", summary.RootServiceName)
	assert.Equal(t, "cycle-a", summary.RootOperationName)
	assert.Zero(t, summary.OrphanSpanCount)
}

func TestFromTraceEmptyTrace(t *testing.T) {
	summary := FromTrace(ptrace.NewTraces())

	assert.True(t, summary.TraceID.IsEmpty())
	assert.Zero(t, summary.SpanCount)
	assert.Empty(t, summary.Services)
}

func BenchmarkFromTrace(b *testing.B) {
	trace := ptrace.NewTraces()
	start := time.Unix(10, 0).UTC()
	for i := 0; i < 1000; i++ {
		parentID := [8]byte{1, 0}
		if i == 0 {
			parentID = [8]byte{}
		}
		status := ptrace.StatusCodeUnset
		if i%10 == 0 {
			status = ptrace.StatusCodeError
		}
		appendSpan(
			trace,
			[]string{"frontend", "backend", "database"}[i%3],
			"operation",
			[8]byte{byte(i>>8) + 1, byte(i)},
			parentID,
			start.Add(time.Duration(i)*time.Microsecond),
			time.Millisecond,
			status,
		)
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = FromTrace(trace)
	}
}

func appendSpan(
	trace ptrace.Traces,
	serviceName string,
	operationName string,
	spanID [8]byte,
	parentSpanID [8]byte,
	start time.Time,
	duration time.Duration,
	status ptrace.StatusCode,
) {
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr(serviceNameAttr, serviceName)
	spans := resourceSpans.ScopeSpans().AppendEmpty().Spans()
	span := spans.AppendEmpty()
	span.SetTraceID(testTraceID)
	span.SetSpanID(pcommon.SpanID(spanID))
	span.SetParentSpanID(pcommon.SpanID(parentSpanID))
	span.SetName(operationName)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(duration)))
	span.Status().SetCode(status)
}
