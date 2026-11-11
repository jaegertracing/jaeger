// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"encoding/binary"
	"os"
	"os/exec"
	"runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestClockSkewAdjuster(t *testing.T) {
	type testSpan struct {
		id, parent          [8]byte
		startTime, duration int
		events              []int // timestamps for logs
		host                string
		adjusted            int   // start time after adjustment
		adjustedEvents      []int // adjusted log timestamps
	}

	toTime := func(t int) time.Time {
		return time.Unix(0, (time.Duration(t) * time.Millisecond).Nanoseconds())
	}

	// helper function that constructs a trace from a list of span prototypes
	makeTrace := func(spanPrototypes []testSpan) ptrace.Traces {
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

	testCases := []struct {
		description string
		trace       []testSpan
		err         string
		maxAdjust   time.Duration
	}{
		{
			description: "single span with bad parent",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0, 0, 0, 0, 0, 0, 0, 99}, startTime: 0, duration: 100, host: "a", adjusted: 0},
			},
			err: "parent span ID=0000000000000063 is not in the trace; skipping clock skew adjustment", // 99 == 0x63
		},
		{
			description: "single span with empty host key",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 0, duration: 100, adjusted: 0},
			},
		},
		{
			description: "two spans with the same ID",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 0, duration: 100, host: "a", adjusted: 0},
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 0, duration: 100, host: "a", adjusted: 0},
			},
			err: "duplicate span IDs; skipping clock skew adjustment",
		},
		{
			description: "parent-child on the same host",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 0, duration: 100, host: "a", adjusted: 0},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 10, duration: 50, host: "a", adjusted: 10},
			},
		},
		{
			description: "do not adjust parent-child on the same host",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 0, duration: 50, host: "a", adjusted: 0},
			},
		},
		{
			description: "do not adjust child that fits inside parent",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 20, duration: 50, host: "b", adjusted: 20},
			},
		},
		{
			description: "do not adjust child that is longer than parent",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 20, duration: 150, host: "b", adjusted: 20},
			},
		},
		{
			description: "do not apply positive adjustment due to max skew adjustment",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 0, duration: 50, host: "b", adjusted: 0},
			},
			maxAdjust: 10 * time.Millisecond,
			err:       "max clock skew adjustment delta of 10ms exceeded; not applying calculated delta of 35ms",
		},
		{
			description: "do not apply negative adjustment due to max skew adjustment",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 80, duration: 50, host: "b", adjusted: 80},
			},
			maxAdjust: 10 * time.Millisecond,
			err:       "max clock skew adjustment delta of 10ms exceeded; not applying calculated delta of -45ms",
		},
		{
			description: "do not apply adjustment due to disabled adjustment",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 0, duration: 50, host: "b", adjusted: 0},
			},
			err: "clock skew adjustment disabled; not applying calculated delta of 35ms",
		},
		{
			description: "adjust child starting before parent",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				// latency = (100-50) / 2 = 25
				// delta = (10 - 0) + latency = 35
				{
					id: [8]byte{2}, parent: [8]byte{1}, startTime: 0, duration: 50, host: "b", adjusted: 35,
					events: []int{5, 10}, adjustedEvents: []int{40, 45},
				},
			},
			maxAdjust: time.Second,
		},
		{
			description: "adjust child starting before parent even if it is longer",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 0, duration: 150, host: "b", adjusted: 10},
			},
			maxAdjust: time.Second,
		},
		{
			description: "adjust child ending after parent but being shorter",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
				// latency: (100 - 70) / 2 = 15
				// new child start time: 10 + latency = 25, delta = -25
				{id: [8]byte{2}, parent: [8]byte{1}, startTime: 50, duration: 70, host: "b", adjusted: 25},
				// same host 'b', so same delta = -25
				// new start time: 60 + delta = 35
				{
					id: [8]byte{3}, parent: [8]byte{2}, startTime: 60, duration: 20, host: "b", adjusted: 35,
					events: []int{65, 70}, adjustedEvents: []int{40, 45},
				},
			},
			maxAdjust: time.Second,
		},
	}

	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.description, func(t *testing.T) {
			trace := makeTrace(testCase.trace)
			adjuster := CorrectClockSkew(tt.maxAdjust)
			adjuster.Adjust(trace)

			var gotErr string
			for i, proto := range testCase.trace {
				id := proto.id
				span := trace.ResourceSpans().At(i).ScopeSpans().At(0).Spans().At(0)
				require.EqualValues(t, proto.id, span.SpanID(), "expecting span with span ID = %d", id)

				warnings := jptrace.GetWarnings(span)
				if testCase.err == "" {
					if proto.adjusted == proto.startTime {
						assert.Empty(t, warnings, "no warnings in span %s", span.SpanID)
					} else {
						assert.Len(t, warnings, 1, "warning about adjutment added to span %s", span.SpanID)
					}
				} else {
					if len(warnings) > 0 {
						gotErr = warnings[0]
					}
				}

				// compare values as int because assert.Equal prints uint64 as hex
				assert.Equal(
					t, toTime(proto.adjusted).UTC(), span.StartTimestamp().AsTime(),
					"adjusted start time of span[ID = %d]", id,
				)

				assert.Equal(
					t, toTime(proto.adjusted+proto.duration).UTC(), span.EndTimestamp().AsTime(),
					"adjusted end time of span[ID = %d]", id,
				)

				for i, logTs := range proto.adjustedEvents {
					assert.Equal(
						t, toTime(logTs).UTC(), span.Events().At(i).Timestamp().AsTime(),
						"adjusted log timestamp of span[ID = %d], log[%d]", id, i,
					)
				}
			}
			assert.Equal(t, testCase.err, gotErr)
		})
	}
}

func TestClockSkewAdjusterRestoresSkewBetweenSiblings(t *testing.T) {
	toTime := func(milliseconds int) time.Time {
		return time.Unix(0, (time.Duration(milliseconds) * time.Millisecond).Nanoseconds()).UTC()
	}
	makeNode := func(id byte, start, duration int, host string) *node {
		span := ptrace.NewSpan()
		span.SetSpanID(pcommon.SpanID([8]byte{id}))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(toTime(start)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(toTime(start + duration)))
		return &node{span: span, hostKey: host}
	}

	root := makeNode(1, 10, 100, "a")
	branch := makeNode(2, 0, 50, "b")
	childC := makeNode(3, 0, 10, "c")
	childB1 := makeNode(4, 5, 10, "b")
	childB2 := makeNode(5, 10, 10, "b")
	root.children = []*node{branch}
	// Children are built from a map, so their order is fixed here to make sure
	// the different-host child is visited before its same-host siblings.
	branch.children = []*node{childC, childB1, childB2}

	adjuster := clockSkewAdjuster{maxDelta: time.Second}
	adjuster.adjustNode(root, nil, clockSkew{hostKey: root.hostKey})

	assert.Equal(t, toTime(10), root.span.StartTimestamp().AsTime())
	assert.Equal(t, toTime(35), branch.span.StartTimestamp().AsTime())
	assert.Equal(t, toTime(55), childC.span.StartTimestamp().AsTime())
	assert.Equal(t, toTime(40), childB1.span.StartTimestamp().AsTime())
	assert.Equal(t, toTime(45), childB2.span.StartTimestamp().AsTime())
}

func TestClockSkewAdjusterDeepTrace(t *testing.T) {
	const childProcessEnv = "JAEGER_TEST_CLOCK_SKEW_DEEP_TRACE"
	if os.Getenv(childProcessEnv) != "1" {
		// Exceeding the maximum stack size is a fatal error that cannot be recovered,
		// so the walk runs in a child process to report it as a test failure.
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
		cmd.Env = append(os.Environ(), childProcessEnv+"=1")
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "deep-trace subprocess failed:\n%s", output)
		return
	}

	toTimestamp := func(milliseconds int) pcommon.Timestamp {
		return pcommon.NewTimestampFromTime(time.UnixMilli(int64(milliseconds)))
	}
	newSpan := func(spans ptrace.SpanSlice, id uint64, parentID pcommon.SpanID, start, duration int) ptrace.Span {
		var spanID pcommon.SpanID
		binary.BigEndian.PutUint64(spanID[:], id)
		span := spans.AppendEmpty()
		span.SetSpanID(spanID)
		span.SetParentSpanID(parentID)
		span.SetStartTimestamp(toTimestamp(start))
		span.SetEndTimestamp(toTimestamp(start + duration))
		return span
	}

	// A single chain of spans on host "a", ending in a span from host "b" that
	// starts before its parent and therefore needs adjustment.
	const depth = 10_000
	traces := ptrace.NewTraces()
	resourceA := traces.ResourceSpans().AppendEmpty()
	resourceA.Resource().Attributes().PutStr("host.name", "a")
	spansA := resourceA.ScopeSpans().AppendEmpty().Spans()
	parentID := pcommon.NewSpanIDEmpty()
	for i := uint64(1); i < depth; i++ {
		parentID = newSpan(spansA, i, parentID, 10, 100).SpanID()
	}
	resourceB := traces.ResourceSpans().AppendEmpty()
	resourceB.Resource().Attributes().PutStr("host.name", "b")
	leaf := newSpan(resourceB.ScopeSpans().AppendEmpty().Spans(), depth, parentID, 0, 50)

	// A recursive walk needs well over 256 KiB of stack for this depth.
	previousMaxStack := debug.SetMaxStack(256 << 10)
	defer debug.SetMaxStack(previousMaxStack)
	CorrectClockSkew(time.Second).Adjust(traces)

	// latency = (100 - 50) / 2 = 25, delta = (10 - 0) + latency = 35
	assert.Equal(t, toTimestamp(35), leaf.StartTimestamp())
	assert.Equal(t, toTimestamp(85), leaf.EndTimestamp())
}

func TestHostKey(t *testing.T) {
	tests := []struct {
		name     string
		resource ptrace.ResourceSpans
		expected string
	}{
		{
			name: "host.id attribute",
			resource: func() ptrace.ResourceSpans {
				rs := ptrace.NewResourceSpans()
				rs.Resource().Attributes().PutStr("host.id", "host-123")
				return rs
			}(),
			expected: "host-123",
		},
		{
			name: "string host.ip attribute",
			resource: func() ptrace.ResourceSpans {
				rs := ptrace.NewResourceSpans()
				rs.Resource().Attributes().PutStr("host.ip", "192.168.1.1")
				return rs
			}(),
			expected: "192.168.1.1",
		},
		{
			name: "slice host.ip attribute",
			resource: func() ptrace.ResourceSpans {
				rs := ptrace.NewResourceSpans()
				addresses := rs.Resource().Attributes().PutEmptySlice("host.ip")
				addresses.AppendEmpty().SetStr("192.168.1.1")
				addresses.AppendEmpty().SetStr("192.168.1.2")
				return rs
			}(),
			expected: "192.168.1.1",
		},
		{
			name: "empty host.ip attribute slice",
			resource: func() ptrace.ResourceSpans {
				rs := ptrace.NewResourceSpans()
				rs.Resource().Attributes().PutEmptySlice("host.ip")
				return rs
			}(),
			expected: "",
		},
		{
			name: "host.name attribute",
			resource: func() ptrace.ResourceSpans {
				rs := ptrace.NewResourceSpans()
				rs.Resource().Attributes().PutStr("host.name", "hostname")
				return rs
			}(),
			expected: "hostname",
		},
		{
			name: "no relevant attributes",
			resource: func() ptrace.ResourceSpans {
				rs := ptrace.NewResourceSpans()
				rs.Resource().Attributes().PutStr("service.name", "service-123")
				return rs
			}(),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hostKey(tt.resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}
