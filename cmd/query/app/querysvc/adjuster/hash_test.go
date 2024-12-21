// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanHash_EmptySpans(t *testing.T) {
	adjuster := SpanHash()
	input := ptrace.NewTraces()
	expected := ptrace.NewTraces()
	adjuster.Adjust(input)
	assert.Equal(t, expected, input)
}

func TestSpanHash_RemovesDuplicateSpans(t *testing.T) {
	adjuster := SpanHash()
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

func TestSpanHash_NoDuplicateSpans(t *testing.T) {
	adjuster := SpanHash()
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

// TODO: write tests for error cases
