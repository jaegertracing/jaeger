// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/mocks"
)

func TestTraceWriter_WriteTraces(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	coreWriter := &mocks.CoreSpanWriter{}
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "testing-service")
	span := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op-1")
	dbSpan := ToDBModel(td)
	writer := TraceWriter{spanWriter: coreWriter, logger: logger}
	coreWriter.On("WriteSpan", model.EpochMicrosecondsAsTime(dbSpan[0].StartTime), &dbSpan[0])
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "wrote spans to ES", logs.All()[0].Message)
	assert.Equal(t, zapcore.DebugLevel, logs.All()[0].Level)
}

func TestTraceWriter_WriteTraces_EmptyTraces(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	coreWriter := &mocks.CoreSpanWriter{}
	writer := TraceWriter{spanWriter: coreWriter, logger: logger}
	td := ptrace.NewTraces()
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	coreWriter.AssertNotCalled(t, "WriteSpan")
	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "skipping write of empty trace data", logs.All()[0].Message)
	assert.Equal(t, zapcore.DebugLevel, logs.All()[0].Level)
}

func TestTraceWriter_WriteTraces_NonEmptyResourceSpansZeroSpans(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	coreWriter := &mocks.CoreSpanWriter{}
	writer := TraceWriter{spanWriter: coreWriter, logger: logger}
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "testing-service")
	rs.ScopeSpans().AppendEmpty() // ScopeSpans present but no actual Spans
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	coreWriter.AssertNotCalled(t, "WriteSpan")
	require.Equal(t, 1, logs.Len())
	logEntry := logs.All()[0]
	assert.Equal(t, "no spans converted from trace data", logEntry.Message)
	assert.Equal(t, zapcore.WarnLevel, logEntry.Level)
	assert.Equal(t, int64(1), logEntry.ContextMap()["resource_spans"])
	assert.Equal(t, int64(1), logEntry.ContextMap()["scope_spans"])
	assert.Equal(t, int64(0), logEntry.ContextMap()["spans"])
}

func TestTraceWriter_Close(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	coreWriter.On("Close").Return(nil)
	writer := TraceWriter{spanWriter: coreWriter, logger: zap.NewNop()}
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

func Test_NewTraceWriter_NilLogger(t *testing.T) {
	params := spanstore.SpanWriterParams{
		MetricsFactory: metrics.NullFactory,
	}
	writer := NewTraceWriter(params)
	assert.NotNil(t, writer)
	assert.NotNil(t, writer.logger)
}
