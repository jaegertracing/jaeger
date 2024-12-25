// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	AddWarnings(span1, "test-warning-1")
	AddWarnings(span1, "test-warning-2")
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
	AddWarnings(span3, "test-warning-3")

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
	traces := ProtoToTraces(batches)

	assert.Equal(t, 2, traces.ResourceSpans().Len())

	spanMap := make(map[string]ptrace.Span)

	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		resource := traces.ResourceSpans().At(i)
		for j := 0; j < resource.ScopeSpans().Len(); j++ {
			scope := resource.ScopeSpans().At(j)
			for k := 0; k < scope.Spans().Len(); k++ {
				span := scope.Spans().At(k)
				spanMap[span.Name()] = span
			}
		}
	}

	span1 := spanMap["test-span-1"]
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}), span1.SpanID())
	assert.Equal(t, []string{"test-warning-1", "test-warning-2"}, GetWarnings(span1))

	span2 := spanMap["test-span-2"]
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}), span2.SpanID())
	assert.Empty(t, GetWarnings(span2))

	span3 := spanMap["test-span-3"]
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}), span3.SpanID())
	assert.Equal(t, []string{"test-warning-3"}, GetWarnings(span3))
}
