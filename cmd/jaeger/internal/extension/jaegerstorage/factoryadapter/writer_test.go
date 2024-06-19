// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	spanstoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestWriteTraces(t *testing.T) {
	memstore := memory.NewStore()
	traceWriter := &TraceWriter{
		spanWriter: memstore,
	}

	td := makeTraces()
	err := traceWriter.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	tdID := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
	traceID, err := model.TraceIDFromBytes(tdID[:])
	require.NoError(t, err)
	trace, err := memstore.GetTrace(context.Background(), traceID)
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Len(t, trace.Spans, 1)
}

func TestWriteTracesError(t *testing.T) {
	mockstore := spanstoreMocks.NewWriter(t)
	mockstore.On(
		"WriteSpan",
		mock.AnythingOfType("context.backgroundCtx"),
		mock.AnythingOfType("*model.Span"),
	).Return(errors.New("mocked error"))

	traceWriter := &TraceWriter{
		spanWriter: mockstore,
	}

	err := traceWriter.WriteTraces(context.Background(), makeTraces())
	require.ErrorContains(t, err, "mocked error")
}

func makeTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	rSpans := traces.ResourceSpans().AppendEmpty()
	sSpans := rSpans.ScopeSpans().AppendEmpty()
	span := sSpans.Spans().AppendEmpty()

	spanID := pcommon.NewSpanIDEmpty()
	spanID[5] = 5 // 0000000000050000
	span.SetSpanID(spanID)

	traceID := pcommon.NewTraceIDEmpty()
	traceID[15] = 1 // 00000000000000000000000000000001
	span.SetTraceID(traceID)

	return traces
}
