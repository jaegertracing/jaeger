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
		resource.Attributes().PutStr("attributeZ", "keyA")
		resource.Attributes().PutStr("attributeA", "keyB")
		resource.Attributes().PutInt("attributeY", 1)
		resource.Attributes().PutStr("attributeX", "keyC")

		ss := rs.ScopeSpans().AppendEmpty()
		span := ss.Spans().AppendEmpty()
		span.Attributes().PutStr("attributeW", "keyD")
		span.Attributes().PutStr("attributeB", "keyZ")
		span.Attributes().PutInt("attributeV", 2)

		event2 := span.Events().AppendEmpty()
		event2.SetName("event2")
		event2.Attributes().PutStr("attributeU", "keyE")
		event2.Attributes().PutStr("attributeT", "keyF")

		event1 := span.Events().AppendEmpty()
		event1.SetName("event1")
		event1.Attributes().PutStr("attributeR", "keyE")
		event1.Attributes().PutStr("attributeS", "keyF")

		return traces
	}
	expected := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()

		resource := rs.Resource()
		resource.Attributes().PutStr("attributeA", "keyB")
		resource.Attributes().PutStr("attributeX", "keyC")
		resource.Attributes().PutInt("attributeY", 1)
		resource.Attributes().PutStr("attributeZ", "keyA")

		ss := rs.ScopeSpans().AppendEmpty()
		span := ss.Spans().AppendEmpty()
		span.Attributes().PutStr("attributeB", "keyZ")
		span.Attributes().PutInt("attributeV", 2)
		span.Attributes().PutStr("attributeW", "keyD")

		event1 := span.Events().AppendEmpty()
		event1.SetName("event1")
		event1.Attributes().PutStr("attributeR", "keyE")
		event1.Attributes().PutStr("attributeS", "keyF")

		event2 := span.Events().AppendEmpty()
		event2.SetName("event2")
		event2.Attributes().PutStr("attributeT", "keyF")
		event2.Attributes().PutStr("attributeU", "keyE")

		return traces
	}

	i := input()
	adjuster.Adjust(i)
	assert.Equal(t, expected(), i)
}
