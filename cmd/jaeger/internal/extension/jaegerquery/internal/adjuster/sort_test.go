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

func TestSortAttributesAndEventsAdjuster(t *testing.T) {
	adjuster := SortCollections()
	input := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()

		resource := rs.Resource()
		resource.Attributes().PutStr("attributeZ", "valA")
		resource.Attributes().PutStr("attributeA", "valB")
		resource.Attributes().PutInt("attributeY", 1)
		resource.Attributes().PutStr("attributeX", "valC")

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().Attributes().PutStr("attributeH", "valI")
		ss.Scope().Attributes().PutStr("attributeF", "valG")

		span := ss.Spans().AppendEmpty()
		span.Attributes().PutStr("attributeW", "valD")
		span.Attributes().PutStr("attributeB", "valZ")
		span.Attributes().PutInt("attributeV", 2)

		event2 := span.Events().AppendEmpty()
		event2.SetName("event2")
		event2.Attributes().PutStr("attributeU", "valE")
		event2.Attributes().PutStr("attributeT", "valF")

		event1 := span.Events().AppendEmpty()
		event1.SetName("event1")
		event1.Attributes().PutStr("attributeR", "valE")
		event1.Attributes().PutStr("attributeS", "valF")

		link1 := span.Links().AppendEmpty()
		link1.Attributes().PutStr("attributeA", "valB")
		link1.Attributes().PutStr("attributeB", "valC")

		link2 := span.Links().AppendEmpty()
		link2.Attributes().PutStr("attributeD", "valE")
		link2.Attributes().PutStr("attributeC", "valD")

		return traces
	}
	expected := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()

		resource := rs.Resource()
		resource.Attributes().PutStr("attributeA", "valB")
		resource.Attributes().PutStr("attributeX", "valC")
		resource.Attributes().PutInt("attributeY", 1)
		resource.Attributes().PutStr("attributeZ", "valA")

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().Attributes().PutStr("attributeF", "valG")
		ss.Scope().Attributes().PutStr("attributeH", "valI")

		span := ss.Spans().AppendEmpty()
		span.Attributes().PutStr("attributeB", "valZ")
		span.Attributes().PutInt("attributeV", 2)
		span.Attributes().PutStr("attributeW", "valD")

		event1 := span.Events().AppendEmpty()
		event1.SetName("event1")
		event1.Attributes().PutStr("attributeR", "valE")
		event1.Attributes().PutStr("attributeS", "valF")

		event2 := span.Events().AppendEmpty()
		event2.SetName("event2")
		event2.Attributes().PutStr("attributeT", "valF")
		event2.Attributes().PutStr("attributeU", "valE")

		link1 := span.Links().AppendEmpty()
		link1.Attributes().PutStr("attributeA", "valB")
		link1.Attributes().PutStr("attributeB", "valC")

		link2 := span.Links().AppendEmpty()
		link2.Attributes().PutStr("attributeC", "valD")
		link2.Attributes().PutStr("attributeD", "valE")

		return traces
	}

	i := input()
	adjuster.Adjust(i)
	assert.Equal(t, expected(), i)
}

func TestSortEvents_ByTimestamp(t *testing.T) {
	adjuster := SortCollections()
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()

	// Append events with names in reverse order of their timestamps
	e3 := span.Events().AppendEmpty()
	e3.SetName("a-third")
	e3.SetTimestamp(pcommon.Timestamp(300))

	e1 := span.Events().AppendEmpty()
	e1.SetName("z-first")
	e1.SetTimestamp(pcommon.Timestamp(100))

	e2 := span.Events().AppendEmpty()
	e2.SetName("m-second")
	e2.SetTimestamp(pcommon.Timestamp(200))

	adjuster.Adjust(traces)

	events := span.Events()
	require.Equal(t, 3, events.Len())

	assert.Equal(t, "z-first", events.At(0).Name())
	assert.Equal(t, pcommon.Timestamp(100), events.At(0).Timestamp())

	assert.Equal(t, "m-second", events.At(1).Name())
	assert.Equal(t, pcommon.Timestamp(200), events.At(1).Timestamp())

	assert.Equal(t, "a-third", events.At(2).Name())
	assert.Equal(t, pcommon.Timestamp(300), events.At(2).Timestamp())
}

func TestSortEvents_TieBreakerByName(t *testing.T) {
	adjuster := SortCollections()
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()

	// Events with identical timestamps must break ties by name in ascending alphabetical order
	e1 := span.Events().AppendEmpty()
	e1.SetName("z-event")
	e1.SetTimestamp(pcommon.Timestamp(500))

	e2 := span.Events().AppendEmpty()
	e2.SetName("b-event")
	e2.SetTimestamp(pcommon.Timestamp(500))

	e3 := span.Events().AppendEmpty()
	e3.SetName("a-event")
	e3.SetTimestamp(pcommon.Timestamp(500))

	adjuster.Adjust(traces)

	events := span.Events()
	require.Equal(t, 3, events.Len())

	assert.Equal(t, "a-event", events.At(0).Name())
	assert.Equal(t, "b-event", events.At(1).Name())
	assert.Equal(t, "z-event", events.At(2).Name())
}
