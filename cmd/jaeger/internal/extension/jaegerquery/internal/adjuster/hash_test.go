// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestSpanHash_EmptySpans(t *testing.T) {
	adjuster := DeduplicateSpans()
	input := ptrace.NewTraces()
	expected := ptrace.NewTraces()
	adjuster.Adjust(input)
	assert.Equal(t, expected, input)
}

func TestSpanHash_RemovesDuplicateSpans(t *testing.T) {
	adjuster := DeduplicateSpans()
	input := func() ptrace.Traces {
		traces := ptrace.NewTraces()

		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("key1", "value1")

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().Attributes().PutStr("key2", "value2")

		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		span2 := spans.AppendEmpty()
		span2.SetName("span2")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{2})

		span3 := spans.AppendEmpty()
		span3.SetName("span1")
		span3.SetTraceID([16]byte{1})
		span3.SetSpanID([8]byte{1})

		span4 := spans.AppendEmpty()
		span4.SetName("span2")
		span4.SetTraceID([16]byte{1})
		span4.SetSpanID([8]byte{2})

		span5 := spans.AppendEmpty()
		span5.SetName("span3")
		span5.SetTraceID([16]byte{3})
		span5.SetSpanID([8]byte{4})

		rs2 := traces.ResourceSpans().AppendEmpty()
		rs2.Resource().Attributes().PutStr("key1", "value1")

		ss2 := rs2.ScopeSpans().AppendEmpty()
		ss2.Scope().Attributes().PutStr("key2", "value2")

		spans2 := ss2.Spans()

		span6 := spans2.AppendEmpty()
		span6.SetName("span4")
		span6.SetTraceID([16]byte{5})
		span6.SetSpanID([8]byte{6})

		span7 := spans2.AppendEmpty()
		span7.SetName("span3")
		span7.SetTraceID([16]byte{3})
		span7.SetSpanID([8]byte{4})

		return traces
	}
	expected := func() ptrace.Traces {
		traces := ptrace.NewTraces()

		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("key1", "value1")

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().Attributes().PutStr("key2", "value2")

		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		span2 := spans.AppendEmpty()
		span2.SetName("span2")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{2})

		span3 := spans.AppendEmpty()
		span3.SetName("span3")
		span3.SetTraceID([16]byte{3})
		span3.SetSpanID([8]byte{4})

		rs2 := traces.ResourceSpans().AppendEmpty()
		rs2.Resource().Attributes().PutStr("key1", "value1")

		ss2 := rs2.ScopeSpans().AppendEmpty()
		ss2.Scope().Attributes().PutStr("key2", "value2")

		spans2 := ss2.Spans()

		span4 := spans2.AppendEmpty()
		span4.SetName("span4")
		span4.SetTraceID([16]byte{5})
		span4.SetSpanID([8]byte{6})

		return traces
	}

	i := input()
	adjuster.Adjust(i)
	assert.Equal(t, expected(), i)
}

func TestSpanHash_NoDuplicateSpans(t *testing.T) {
	adjuster := DeduplicateSpans()
	input := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		span2 := spans.AppendEmpty()
		span2.SetName("span2")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{2})

		span3 := spans.AppendEmpty()
		span3.SetName("span3")
		span3.SetTraceID([16]byte{3})
		span3.SetSpanID([8]byte{4})

		return traces
	}
	expected := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		span2 := spans.AppendEmpty()
		span2.SetName("span2")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{2})

		span3 := spans.AppendEmpty()
		span3.SetName("span3")
		span3.SetTraceID([16]byte{3})
		span3.SetSpanID([8]byte{4})

		return traces
	}

	i := input()
	adjuster.Adjust(i)
	assert.Equal(t, expected(), i)
}

func TestSpanHash_DuplicateSpansDifferentScopeAttributes(t *testing.T) {
	adjuster := DeduplicateSpans()
	input := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().Attributes().PutStr("key", "value1")
		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		ss2 := rs.ScopeSpans().AppendEmpty()
		ss2.Scope().Attributes().PutStr("key", "value2")
		spans2 := ss2.Spans()

		span2 := spans2.AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}
	expected := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().Attributes().PutStr("key", "value1")
		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		ss2 := rs.ScopeSpans().AppendEmpty()
		ss2.Scope().Attributes().PutStr("key", "value2")
		spans2 := ss2.Spans()

		span2 := spans2.AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}

	i := input()
	adjuster.Adjust(i)
	assert.Equal(t, expected(), i)
}

func TestSpanHash_DuplicateSpansDifferentResourceAttributes(t *testing.T) {
	adjuster := DeduplicateSpans()
	input := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("key", "value1")
		ss := rs.ScopeSpans().AppendEmpty()
		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		rs2 := traces.ResourceSpans().AppendEmpty()
		rs2.Resource().Attributes().PutStr("key", "value2")
		ss2 := rs2.ScopeSpans().AppendEmpty()
		spans2 := ss2.Spans()

		span2 := spans2.AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}
	expected := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("key", "value1")
		ss := rs.ScopeSpans().AppendEmpty()
		spans := ss.Spans()

		span1 := spans.AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		rs2 := traces.ResourceSpans().AppendEmpty()
		rs2.Resource().Attributes().PutStr("key", "value2")
		ss2 := rs2.ScopeSpans().AppendEmpty()
		spans2 := ss2.Spans()

		span2 := spans2.AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}

	i := input()
	adjuster.Adjust(i)
	assert.Equal(t, expected(), i)
}

func TestSpanHash_DuplicateSpansDifferentScopeName(t *testing.T) {
	adjuster := DeduplicateSpans()
	makeTraces := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("scope-a")
		span1 := ss.Spans().AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		ss2 := rs.ScopeSpans().AppendEmpty()
		ss2.Scope().SetName("scope-b")
		span2 := ss2.Spans().AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}

	i := makeTraces()
	adjuster.Adjust(i)
	// The two spans belong to different instrumentation scopes, so they are
	// distinct and must both be preserved.
	assert.Equal(t, makeTraces(), i)
}

func TestSpanHash_DuplicateSpansDifferentScopeVersion(t *testing.T) {
	adjuster := DeduplicateSpans()
	makeTraces := func() ptrace.Traces {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("scope")
		ss.Scope().SetVersion("v1")
		span1 := ss.Spans().AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		ss2 := rs.ScopeSpans().AppendEmpty()
		ss2.Scope().SetName("scope")
		ss2.Scope().SetVersion("v2")
		span2 := ss2.Spans().AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}

	i := makeTraces()
	adjuster.Adjust(i)
	assert.Equal(t, makeTraces(), i)
}

func TestSpanHash_DuplicateSpansDifferentSchemaURL(t *testing.T) {
	adjuster := DeduplicateSpans()
	makeTraces := func() ptrace.Traces {
		traces := ptrace.NewTraces()

		rs := traces.ResourceSpans().AppendEmpty()
		rs.SetSchemaUrl("https://example.com/schema/1")
		ss := rs.ScopeSpans().AppendEmpty()
		ss.SetSchemaUrl("https://example.com/scope/1")
		span1 := ss.Spans().AppendEmpty()
		span1.SetName("span1")
		span1.SetTraceID([16]byte{1})
		span1.SetSpanID([8]byte{1})

		rs2 := traces.ResourceSpans().AppendEmpty()
		rs2.SetSchemaUrl("https://example.com/schema/2")
		ss2 := rs2.ScopeSpans().AppendEmpty()
		ss2.SetSchemaUrl("https://example.com/scope/2")
		span2 := ss2.Spans().AppendEmpty()
		span2.SetName("span1")
		span2.SetTraceID([16]byte{1})
		span2.SetSpanID([8]byte{1})

		return traces
	}

	i := makeTraces()
	adjuster.Adjust(i)
	// Spans carrying different resource/scope schema URLs are distinct and must
	// both survive.
	assert.Equal(t, makeTraces(), i)
}

type errorMarshaler struct{}

func (*errorMarshaler) MarshalTraces(ptrace.Traces) ([]byte, error) {
	return nil, assert.AnError
}

func TestSpanHash_ErrorInMarshaler(t *testing.T) {
	adjuster := SpanHashDeduper{
		marshaler: &errorMarshaler{},
	}

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	spans := ss.Spans()

	span := spans.AppendEmpty()
	span.SetName("span1")

	adjuster.Adjust(traces)

	gotSpan := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, "span1", gotSpan.Name())

	warnings := jptrace.GetWarnings(gotSpan)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "failed to compute hash code")
	assert.Contains(t, warnings[0], assert.AnError.Error())
}
