// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestLinksAdjuster(t *testing.T) {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	// span with no links
	scopeSpans.Spans().AppendEmpty()

	// span with empty traceID link
	spanB := scopeSpans.Spans().AppendEmpty()
	spanB.Links().AppendEmpty().SetTraceID(pcommon.NewTraceIDEmpty())

	// span with 2 non-empty traceID links and 1 empty (or zero) traceID link
	spanC := scopeSpans.Spans().AppendEmpty()
	spanC.Links().AppendEmpty().SetTraceID(pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	spanC.Links().AppendEmpty().SetTraceID(pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}))
	spanC.Links().AppendEmpty().SetTraceID(pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))

	err := SpanLinks().Adjust(traces)
	spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.NoError(t, err)

	gotSpansA := spans.At(0)
	assert.Equal(t, 0, gotSpansA.Links().Len())
	assert.Empty(t, jptrace.GetWarnings(gotSpansA))

	gotSpansB := spans.At(1)
	assert.Equal(t, 0, gotSpansB.Links().Len())
	spanBWarnings := jptrace.GetWarnings(gotSpansB)
	assert.Len(t, spanBWarnings, 1)
	assert.Equal(t, "Invalid span link removed", spanBWarnings[0])

	gotSpansC := spans.At(2)
	assert.Equal(t, 2, gotSpansC.Links().Len())
	spanCWarnings := jptrace.GetWarnings(gotSpansC)
	assert.Len(t, spanCWarnings, 1)
	assert.Equal(t, "Invalid span link removed", spanCWarnings[0])
}
