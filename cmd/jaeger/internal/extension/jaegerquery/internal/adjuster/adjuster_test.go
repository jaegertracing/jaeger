// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type mockAdjuster struct{}

func (mockAdjuster) Adjust(traces ptrace.Traces) {
	span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	spanId := span.SpanID()
	spanId[7]++
	span.SetSpanID(spanId)
}

func TestSequences(t *testing.T) {
	trace := ptrace.NewTraces()
	span := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 0})

	a := Sequence(mockAdjuster{}, mockAdjuster{})

	a.Adjust(trace)

	adjTraceSpan := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, span, adjTraceSpan)
	assert.EqualValues(t, [8]byte{0, 0, 0, 0, 0, 0, 0, 2}, span.SpanID())
}

type testSpan struct {
	id, parent          [8]byte
	startTime, duration int
	events              []int // timestamps for logs
	host                string
	adjusted            int   // start time after adjustment
	adjustedEvents      []int // adjusted log timestamps
}

func makeTrace(spanPrototypes []testSpan) ptrace.Traces {
	trace := ptrace.NewTraces()
	for _, spanProto := range spanPrototypes {
		traceID := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 1})
		span := ptrace.NewSpan()
		span.SetTraceID(traceID)
		span.SetSpanID(spanProto.id)
		span.SetParentSpanID(spanProto.parent)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(toTime(spanProto.startTime)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(toTime(spanProto.startTime + spanProto.duration)))

		events := ptrace.NewSpanEventSlice()
		for _, log := range spanProto.events {
			event := events.AppendEmpty()
			event.SetTimestamp(pcommon.NewTimestampFromTime(toTime(log)))
			event.Attributes().PutStr("event", "some event")
		}
		events.CopyTo(span.Events())

		resource := ptrace.NewResourceSpans()
		resource.Resource().Attributes().PutEmptySlice("host.ip").AppendEmpty().SetStr(spanProto.host)

		span.CopyTo(resource.ScopeSpans().AppendEmpty().Spans().AppendEmpty())
		resource.CopyTo(trace.ResourceSpans().AppendEmpty())
	}
	return trace
}

func toTime(t int) time.Time {
	return time.Unix(0, (time.Duration(t) * time.Millisecond).Nanoseconds())
}
