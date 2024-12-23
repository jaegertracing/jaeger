package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestProtoFromTraces_Warnings(t *testing.T) {
	traces := ptrace.NewTraces()
	rs1 := traces.ResourceSpans().AppendEmpty()
	ss1 := rs1.ScopeSpans().AppendEmpty()
	span1 := ss1.Spans().AppendEmpty()
	span1.SetName("test-span-1")
	AddWarning(span1, "test-warning-1")
	AddWarning(span1, "test-warning-2")

	ss2 := rs1.ScopeSpans().AppendEmpty()
	span2 := ss2.Spans().AppendEmpty()
	span2.SetName("test-span-2")

	rs2 := traces.ResourceSpans().AppendEmpty()
	ss3 := rs2.ScopeSpans().AppendEmpty()
	span3 := ss3.Spans().AppendEmpty()
	span3.SetName("test-span-3")
	AddWarning(span3, "test-warning-3")

	ss4 := rs2.ScopeSpans().AppendEmpty()
	span4 := ss4.Spans().AppendEmpty()
	span4.SetName("test-span-4")
	AddWarning(span4, "test-warning-4")

	batches := ProtoFromTraces(traces)

	assert.Len(t, batches, 2)

	assert.Len(t, batches[0].Spans, 2)
	assert.Equal(t, "test-span-1", batches[0].Spans[0].OperationName)
	assert.Equal(t, []string{"test-warning-1", "test-warning-2"}, batches[0].Spans[0].Warnings)
	assert.Equal(t, "test-span-2", batches[0].Spans[1].OperationName)
	assert.Empty(t, batches[0].Spans[1].Warnings)

	assert.Len(t, batches[1].Spans, 2)
	assert.Equal(t, "test-span-3", batches[1].Spans[0].OperationName)
	assert.Equal(t, []string{"test-warning-3"}, batches[1].Spans[0].Warnings)
	assert.Equal(t, "test-span-4", batches[1].Spans[1].OperationName)
	assert.Equal(t, []string{"test-warning-4"}, batches[1].Spans[1].Warnings)
}
