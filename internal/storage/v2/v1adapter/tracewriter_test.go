// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func TestWriteTraces(t *testing.T) {
	f := badger.NewFactory()
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	traceWriter := &TraceWriter{
		spanWriter: spanWriter,
	}

	td := makeTraces()
	err = traceWriter.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	tdID := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
	traceID, err := model.TraceIDFromBytes(tdID[:])
	require.NoError(t, err)
	query := spanstore.GetTraceParameters{TraceID: traceID}
	trace, err := spanReader.GetTrace(context.Background(), query)
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Len(t, trace.Spans, 1)
}

func TestWriteTracesError(t *testing.T) {
	mockstore := spanstoremocks.NewWriter(t)
	mockstore.On(
		"WriteSpan",
		mock.Anything,
		mock.AnythingOfType("*model.Span"),
	).Return(errors.New("mocked error"))

	traceWriter := &TraceWriter{
		spanWriter: mockstore,
	}

	err := traceWriter.WriteTraces(context.Background(), makeTraces())
	require.ErrorContains(t, err, "mocked error")
}

func TestWriteTraces_ContextCancellation(t *testing.T) {
	// Use an empty mock; if WriteSpan is called, it will panic/fail the test,
	// which is what we want since the context is pre-canceled.
	mockstore := new(spanstoremocks.Writer)

	traceWriter := &TraceWriter{
		spanWriter: mockstore,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	err := traceWriter.WriteTraces(ctx, makeTraces())
	require.ErrorIs(t, err, context.Canceled)
}

func TestGetV1Writer(t *testing.T) {
	t.Run("wrapped v1 writer", func(t *testing.T) {
		writer := new(spanstoremocks.Writer)
		traceWriter := &TraceWriter{
			spanWriter: writer,
		}
		v1Writer := GetV1Writer(traceWriter)
		require.Equal(t, writer, v1Writer)
	})

	t.Run("native v2 writer", func(t *testing.T) {
		writer := new(tracestoremocks.Writer)
		v1Writer := GetV1Writer(writer)
		require.IsType(t, &SpanWriter{}, v1Writer)
		require.Equal(t, writer, v1Writer.(*SpanWriter).traceWriter)
	})
}

func TestWriteDependencies(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deps := []model.DependencyLink{
		{Parent: "a", Child: "b", CallCount: 1},
	}
	m := depstoremocks.NewWriter(t)
	m.EXPECT().WriteDependencies(ts, deps).Return(nil)
	dw := NewDependencyWriter(m)
	err := dw.WriteDependencies(context.Background(), ts, deps)
	require.NoError(t, err)
}

func TestWriteDependencies_Error(t *testing.T) {
	m := depstoremocks.NewWriter(t)
	m.EXPECT().WriteDependencies(mock.Anything, mock.Anything).Return(assert.AnError)
	dw := NewDependencyWriter(m)
	err := dw.WriteDependencies(context.Background(), time.Now(), nil)
	require.ErrorIs(t, err, assert.AnError)
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
