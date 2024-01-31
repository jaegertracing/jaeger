// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestBatchesToTraces(t *testing.T) {
	b1 := &model.Batch{
		Spans: []*model.Span{
			{TraceID: model.NewTraceID(1, 2), SpanID: model.NewSpanID(1), OperationName: "x"},
			{TraceID: model.NewTraceID(1, 3), SpanID: model.NewSpanID(2), OperationName: "y"},
		},
		Process: model.NewProcess("process1", model.KeyValues{}),
	}

	b2 := &model.Batch{
		Spans: []*model.Span{
			{TraceID: model.NewTraceID(1, 2), SpanID: model.NewSpanID(2), OperationName: "z"},
		},
		Process: model.NewProcess("process2", model.KeyValues{}),
	}

	mainBatch := []*model.Batch{b1, b2}

	traces, err := batchesToTraces(mainBatch)
	require.Nil(t, err)

	s1 := []*model.Span{
		{
			TraceID:       model.NewTraceID(1, 2),
			SpanID:        model.NewSpanID(1),
			OperationName: "x",
			Process:       model.NewProcess("process1", model.KeyValues{}),
		},
		{
			TraceID:       model.NewTraceID(1, 2),
			SpanID:        model.NewSpanID(2),
			OperationName: "z",
			Process:       model.NewProcess("process2", model.KeyValues{}),
		},
	}

	s2 := []*model.Span{
		{
			TraceID:       model.NewTraceID(1, 3),
			SpanID:        model.NewSpanID(2),
			OperationName: "y",
			Process:       model.NewProcess("process1", model.KeyValues{}),
		},
	}

	t1 := model.Trace{
		Spans: s1,
	}
	t2 := model.Trace{
		Spans: s2,
	}
	mainTrace := []*model.Trace{&t1, &t2}
	assert.Equal(t, mainTrace, traces)
}
