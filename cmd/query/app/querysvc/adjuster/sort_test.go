// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSortAttributesAndEventsAdjuster(t *testing.T) {
	adjuster := SortAttributesAndEvents()
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
