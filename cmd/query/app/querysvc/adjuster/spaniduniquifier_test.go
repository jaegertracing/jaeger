// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var (
	clientSpanID  = pcommon.SpanID([]byte{0, 0, 0, 0, 0, 0, 0, 1})
	anotherSpanID = pcommon.SpanID([]byte{1, 0, 0, 0, 0, 0, 0, 0})
)

func makeTraces() ptrace.Traces {
	traceID := pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3})

	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()

	clientSpan := spans.AppendEmpty()
	clientSpan.SetTraceID(traceID)
	clientSpan.SetSpanID(clientSpanID)
	clientSpan.SetKind(ptrace.SpanKindClient)

	serverSpan := spans.AppendEmpty()
	serverSpan.SetTraceID(traceID)
	serverSpan.SetSpanID(clientSpanID) // shared span ID
	serverSpan.SetKind(ptrace.SpanKindServer)

	anotherSpan := spans.AppendEmpty()
	anotherSpan.SetTraceID(traceID)
	anotherSpan.SetSpanID(anotherSpanID)

	return traces
}

func TestSpanIDUniquifierTriggered(t *testing.T) {
	trc := makeTraces()
	deduper := SpanIDUniquifier()
	traces, err := deduper.Adjust(trc)
	require.NoError(t, err)

	spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	clientSpan := spans.At(0)
	assert.Equal(t, clientSpanID, clientSpan.SpanID(), "client span ID should not change")

	serverSpan := spans.At(1)
	assert.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 2}, serverSpan.SpanID(), "server span ID should be reassigned")
	assert.Equal(t, clientSpan.SpanID(), serverSpan.ParentSpanID(), "client span should be server span's parent")

	thirdSpan := spans.At(2)
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID(), "3rd span ID should not change")
}

func TestSpanIDUniquifierNotTriggered(t *testing.T) {
	trc := makeTraces()
	spans := trc.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// remove client span
	newSpans := ptrace.NewSpanSlice()
	spans.At(1).CopyTo(newSpans.AppendEmpty())
	spans.At(2).CopyTo(newSpans.AppendEmpty())
	newSpans.CopyTo(spans)

	deduper := SpanIDUniquifier()
	traces, err := deduper.Adjust(trc)
	require.NoError(t, err)

	gotSpans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	serverSpanID := clientSpanID // for better readability
	serverSpan := gotSpans.At(0)
	assert.Equal(t, serverSpanID, serverSpan.SpanID(), "server span ID should be unchanged")

	thirdSpan := gotSpans.At(1)
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID(), "3rd span ID should not change")
}

func TestSpanIDUniquifierError(t *testing.T) {
	trc := makeTraces()

	maxID := pcommon.SpanID([8]byte{255, 255, 255, 255, 255, 255, 255, 255})

	deduper := &SpanIDDeduper{maxUsedID: maxID}
	traces, err := deduper.Adjust(trc)
	require.NoError(t, err)

	span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(1)
	val, ok := span.Attributes().Get(adjusterWarningAttribute)
	require.True(t, ok)
	require.Equal(t, "cannot assign unique span ID, too many spans in the trace", val.Slice().At(0).Str())
}
