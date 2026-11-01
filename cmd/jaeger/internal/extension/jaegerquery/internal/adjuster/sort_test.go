// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		event2.SetTimestamp(pcommon.Timestamp(200))
		event2.Attributes().PutStr("attributeU", "valE")
		event2.Attributes().PutStr("attributeT", "valF")

		event1 := span.Events().AppendEmpty()
		event1.SetName("event1")
		event1.SetTimestamp(pcommon.Timestamp(100))
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
		event1.SetTimestamp(pcommon.Timestamp(100))
		event1.Attributes().PutStr("attributeR", "valE")
		event1.Attributes().PutStr("attributeS", "valF")

		event2 := span.Events().AppendEmpty()
		event2.SetName("event2")
		event2.SetTimestamp(pcommon.Timestamp(200))
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

// TestSortEventsByTimestamp uses names whose lexicographic order is the
// reverse of their timestamps, so it fails if events are sorted by name.
func TestSortEventsByTimestamp(t *testing.T) {
	adjuster := SortCollections()
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	// Appended out of order; names sort as a-late, m-middle, z-early.
	type event struct {
		name      string
		timestamp pcommon.Timestamp
	}
	events := []event{
		{name: "a-late", timestamp: 300},
		{name: "z-early", timestamp: 100},
		{name: "m-middle", timestamp: 200},
	}
	for _, e := range events {
		added := span.Events().AppendEmpty()
		added.SetName(e.name)
		added.SetTimestamp(e.timestamp)
	}

	adjuster.Adjust(traces)

	got := span.Events()
	assert.Equal(t, 3, got.Len())

	// Chronological order, which is the opposite of the name order above.
	assert.Equal(t, "z-early", got.At(0).Name())
	assert.Equal(t, "m-middle", got.At(1).Name())
	assert.Equal(t, "a-late", got.At(2).Name())

	for i := 1; i < got.Len(); i++ {
		assert.Less(t, got.At(i-1).Timestamp(), got.At(i).Timestamp())
	}
}

// TestSortEventsBreaksTimestampTiesByName covers events recorded at the same
// instant, where the timestamp alone cannot order them. Appending them in an
// order that differs from their names makes the tie-break visible.
func TestSortEventsBreaksTimestampTiesByName(t *testing.T) {
	adjuster := SortCollections()
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	for _, name := range []string{"b", "a", "c"} {
		added := span.Events().AppendEmpty()
		added.SetName(name)
		added.SetTimestamp(pcommon.Timestamp(500))
	}

	adjuster.Adjust(traces)

	got := span.Events()
	assert.Equal(t, 3, got.Len())
	assert.Equal(t, "a", got.At(0).Name())
	assert.Equal(t, "b", got.At(1).Name())
	assert.Equal(t, "c", got.At(2).Name())
}
