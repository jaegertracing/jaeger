// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
)

func TestProtoFromTraces_AddsWarnings(t *testing.T) {
	traces := ptrace.NewTraces()
	rs1 := traces.ResourceSpans().AppendEmpty()
	ss1 := rs1.ScopeSpans().AppendEmpty()
	span1 := ss1.Spans().AppendEmpty()
	span1.SetName("test-span-1")
	span1.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	AddWarning(span1, "test-warning-1")
	AddWarning(span1, "test-warning-2")
	span1.Attributes().PutStr("key", "value")

	ss2 := rs1.ScopeSpans().AppendEmpty()
	span2 := ss2.Spans().AppendEmpty()
	span2.SetName("test-span-2")
	span2.SetSpanID(pcommon.SpanID([8]byte{9, 10, 11, 12, 13, 14, 15, 16}))

	rs2 := traces.ResourceSpans().AppendEmpty()
	ss3 := rs2.ScopeSpans().AppendEmpty()
	span3 := ss3.Spans().AppendEmpty()
	span3.SetName("test-span-3")
	span3.SetSpanID(pcommon.SpanID([8]byte{17, 18, 19, 20, 21, 22, 23, 24}))
	AddWarning(span3, "test-warning-3")

	batches := ProtoFromTraces(traces)

	assert.Len(t, batches, 2)

	assert.Len(t, batches[0].Spans, 2)
	assert.Equal(t, "test-span-1", batches[0].Spans[0].OperationName)
	assert.Equal(t, []string{"test-warning-1", "test-warning-2"}, batches[0].Spans[0].Warnings)
	assert.Equal(t, []model.KeyValue{{Key: "key", VStr: "value"}}, batches[0].Spans[0].Tags)
	assert.Equal(t, "test-span-2", batches[0].Spans[1].OperationName)
	assert.Empty(t, batches[0].Spans[1].Warnings)
	assert.Empty(t, batches[0].Spans[1].Tags)

	assert.Len(t, batches[1].Spans, 1)
	assert.Equal(t, "test-span-3", batches[1].Spans[0].OperationName)
	assert.Equal(t, []string{"test-warning-3"}, batches[1].Spans[0].Warnings)
	assert.Empty(t, batches[1].Spans[0].Tags)
}

func TestProtoToTraces_AddsWarnings(t *testing.T) {
	batch1 := &model.Batch{
		Process: &model.Process{
			ServiceName: "batch-1",
		},
		Spans: []*model.Span{
			{
				OperationName: "test-span-1",
				SpanID:        model.NewSpanID(1),
				Warnings:      []string{"test-warning-1", "test-warning-2"},
			},
			{
				OperationName: "test-span-2",
				SpanID:        model.NewSpanID(2),
			},
		},
	}
	batch2 := &model.Batch{
		Process: &model.Process{
			ServiceName: "batch-2",
		},
		Spans: []*model.Span{
			{
				OperationName: "test-span-3",
				SpanID:        model.NewSpanID(3),
				Warnings:      []string{"test-warning-3"},
			},
		},
	}
	batches := []*model.Batch{batch1, batch2}
	traces, err := ProtoToTraces(batches)
	require.NoError(t, err)

	assert.Equal(t, 2, traces.ResourceSpans().Len())

	rs1 := traces.ResourceSpans().At(0)
	assert.Equal(t, 1, rs1.ScopeSpans().Len())
	ss1 := rs1.ScopeSpans().At(0)
	assert.Equal(t, 2, ss1.Spans().Len())

	span1 := ss1.Spans().At(0)
	assert.Equal(t, "test-span-1", span1.Name())
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}), span1.SpanID())
	assert.Equal(t, []string{"test-warning-1", "test-warning-2"}, GetWarnings(span1))

	span2 := ss1.Spans().At(1)
	assert.Equal(t, "test-span-2", span2.Name())
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}), span2.SpanID())
	assert.Empty(t, GetWarnings(span2))

	rs2 := traces.ResourceSpans().At(1)
	assert.Equal(t, 1, rs2.ScopeSpans().Len())
	ss3 := rs2.ScopeSpans().At(0)
	assert.Equal(t, 1, ss3.Spans().Len())

	span3 := ss3.Spans().At(0)
	assert.Equal(t, "test-span-3", span3.Name())
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}), span3.SpanID())
	assert.Equal(t, []string{"test-warning-3"}, GetWarnings(span3))
}
