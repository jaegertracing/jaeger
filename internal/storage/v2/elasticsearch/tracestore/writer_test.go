// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
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
	dbSpan := ToDBModel(td)
	writer := TraceWriter{spanWriter: coreWriter}
	coreWriter.On("WriteSpan", model.EpochMicrosecondsAsTime(dbSpan[0].StartTime), &dbSpan[0]).Return(nil)
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
}

func TestTraceWriter_Close(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	coreWriter.On("Close").Return(nil)
	writer := TraceWriter{spanWriter: coreWriter}
	err := writer.Close()
	require.NoError(t, err)
}

func TestTraceWriter_CreateTemplates(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	coreWriter.On("CreateTemplates", "testing-template", "testing-template", cfg.IndexPrefix("testing")).Return(nil)
	writer := TraceWriter{spanWriter: coreWriter}
	err := writer.CreateTemplates("testing-template", "testing-template", "testing")
	require.NoError(t, err)
}

func Test_NewTraceWriter(t *testing.T) {
	params := spanstore.SpanWriterParams{}
	writer := NewTraceWriter(params)
	assert.NotNil(t, writer)
}
