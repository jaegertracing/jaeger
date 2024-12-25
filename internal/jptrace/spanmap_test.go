package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanMap(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span1 := ss.Spans().AppendEmpty()
	span1.SetName("span1")
	span2 := ss.Spans().AppendEmpty()
	span2.SetName("span2")

	keyFn := func(span ptrace.Span) string {
		return span.Name()
	}

	spanMap := SpanMap(traces, keyFn)

	expectedMap := map[string]ptrace.Span{
		"span1": span1,
		"span2": span2,
	}
	assert.Equal(t, expectedMap, spanMap)
}
