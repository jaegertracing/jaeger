// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanIter(t *testing.T) {
	traces := ptrace.NewTraces()

	resource1 := traces.ResourceSpans().AppendEmpty()
	scope1 := resource1.ScopeSpans().AppendEmpty()

	span1 := scope1.Spans().AppendEmpty()
	span1.SetName("span-1")

	span2 := scope1.Spans().AppendEmpty()
	span2.SetName("span-2")

	resource2 := traces.ResourceSpans().AppendEmpty()
	scope2 := resource2.ScopeSpans().AppendEmpty()

	span3 := scope2.Spans().AppendEmpty()
	span3.SetName("span-3")

	scope3 := resource2.ScopeSpans().AppendEmpty()

	span4 := scope3.Spans().AppendEmpty()
	span4.SetName("span-4")

	spanIter := SpanIter(traces)
	var spans []ptrace.Span
	var positions []SpanIterPos
	spanIter(func(pos SpanIterPos, span ptrace.Span) bool {
		spans = append(spans, span)
		positions = append(positions, pos)
		return true
	})

	assert.Len(t, spans, 4)
	assert.Equal(t, "span-1", spans[0].Name())
	assert.Equal(t, "span-2", spans[1].Name())
	assert.Equal(t, "span-3", spans[2].Name())
	assert.Equal(t, "span-4", spans[3].Name())

	assert.Len(t, positions, 4)
	assert.Equal(t, 0, positions[0].ResourceIndex)
	assert.Equal(t, resource1, positions[0].Resource)
	assert.Equal(t, 0, positions[0].ScopeIndex)
	assert.Equal(t, scope1, positions[0].Scope)

	assert.Equal(t, 0, positions[1].ResourceIndex)
	assert.Equal(t, resource1, positions[1].Resource)
	assert.Equal(t, 0, positions[1].ScopeIndex)
	assert.Equal(t, scope1, positions[1].Scope)

	assert.Equal(t, 1, positions[2].ResourceIndex)
	assert.Equal(t, resource2, positions[2].Resource)
	assert.Equal(t, 0, positions[2].ScopeIndex)
	assert.Equal(t, scope2, positions[2].Scope)

	assert.Equal(t, 1, positions[3].ResourceIndex)
	assert.Equal(t, resource2, positions[3].Resource)
	assert.Equal(t, 1, positions[3].ScopeIndex)
	assert.Equal(t, scope3, positions[3].Scope)
}

func TestSpanIterStopIteration(t *testing.T) {
	traces := ptrace.NewTraces()

	resource1 := traces.ResourceSpans().AppendEmpty()
	scope1 := resource1.ScopeSpans().AppendEmpty()

	span1 := scope1.Spans().AppendEmpty()
	span1.SetName("span-1")

	span2 := scope1.Spans().AppendEmpty()
	span2.SetName("span-2")

	spanIter := SpanIter(traces)
	var spans []ptrace.Span
	spanIter(func(_ SpanIterPos, span ptrace.Span) bool {
		spans = append(spans, span)
		return false
	})

	assert.Len(t, spans, 1)
	assert.Equal(t, "span-1", spans[0].Name())
}

func TestGetTraceID(t *testing.T) {
	t.Run("empty traces returns empty TraceID", func(t *testing.T) {
		traces := ptrace.NewTraces()
		assert.Equal(t, pcommon.NewTraceIDEmpty(), GetTraceID(traces))
	})

	t.Run("returns TraceID of first span", func(t *testing.T) {
		traces := ptrace.NewTraces()
		span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		traceID := pcommon.TraceID([16]byte{1, 2, 3})
		span.SetTraceID(traceID)
		assert.Equal(t, traceID, GetTraceID(traces))
	})
}
