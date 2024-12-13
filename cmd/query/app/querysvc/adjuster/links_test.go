// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestLinksAdjuster(t *testing.T) {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	// span with no links
	scopeSpans.Spans().AppendEmpty()

	// span with empty traceID link
	spanA := scopeSpans.Spans().AppendEmpty()
	spanA.Links().AppendEmpty().SetTraceID(pcommon.NewTraceIDEmpty())

	// span with 2 non-empty traceID links and 1 empty (or zero) traceID link
	spanB := scopeSpans.Spans().AppendEmpty()
	spanB.Links().AppendEmpty().SetTraceID(pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	spanB.Links().AppendEmpty().SetTraceID(pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}))
	spanB.Links().AppendEmpty().SetTraceID(pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))

	trace, err := Links().Adjust(trace)
	spans := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.NoError(t, err)
	assert.Equal(t, 0, spans.At(0).Links().Len())
	assert.Equal(t, 0, spans.At(1).Links().Len())
	assert.Equal(t, 2, spans.At(2).Links().Len())
}
