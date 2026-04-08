// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/mocks"
)

func TestTraceWriter_WriteTraces(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "testing-service")
	span := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op-1")
	dbSpans := ToDBModel(td)
	writer := TraceWriter{spanWriter: coreWriter}

	spans := make([]*dbmodel.Span, len(dbSpans))
	startTimes := make([]time.Time, len(dbSpans))
	for i := range dbSpans {
		spans[i] = &dbSpans[i]
		startTimes[i] = model.EpochMicrosecondsAsTime(dbSpans[i].StartTime)
	}

	coreWriter.On("WriteSpansSync", context.Background(), spans, startTimes).Return(nil)
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
}

func TestTraceWriter_WriteTraces_Error(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "testing-service")
	span := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op-1")
	dbSpans := ToDBModel(td)
	writer := TraceWriter{spanWriter: coreWriter}

	spans := make([]*dbmodel.Span, len(dbSpans))
	startTimes := make([]time.Time, len(dbSpans))
	for i := range dbSpans {
		spans[i] = &dbSpans[i]
		startTimes[i] = model.EpochMicrosecondsAsTime(dbSpans[i].StartTime)
	}

	coreWriter.On("WriteSpansSync", context.Background(), spans, startTimes).Return(assert.AnError)
	err := writer.WriteTraces(context.Background(), td)
	require.ErrorIs(t, err, assert.AnError)
}

func TestTraceWriter_WriteTraces_Empty(t *testing.T) {
	writer := TraceWriter{spanWriter: &mocks.CoreSpanWriter{}}
	err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
	require.NoError(t, err)
}

func TestTraceWriter_Close(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	coreWriter.On("Close").Return(nil)
	writer := TraceWriter{spanWriter: coreWriter}
	err := writer.Close()
	require.NoError(t, err)
}

func Test_NewTraceWriter(t *testing.T) {
	params := spanstore.SpanWriterParams{
		Logger:         zap.NewNop(),
		MetricsFactory: metrics.NullFactory,
	}
	writer := NewTraceWriter(params)
	assert.NotNil(t, writer)
}
