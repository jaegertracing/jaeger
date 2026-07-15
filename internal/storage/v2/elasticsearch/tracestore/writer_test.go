// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/mocks"
)

func TestTraceWriter_WriteTraces(t *testing.T) {
	coreWriter := &mocks.Writer{}
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "testing-service")
	span := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op-1")
	dbSpans := ToDBModel(td)
	writer := TraceWriter{spanWriter: coreWriter}
	coreWriter.On("WriteSpans", mock.Anything, dbSpans).Return(nil)
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
}

// TestTraceWriter_WriteTraces_OmitParentSpanIDReference pins that enabling the
// omitParentSpanIDReference gate makes WriteTraces write spans whose parent span ID
// is not also encoded as a synthetic CHILD_OF reference.
func TestTraceWriter_WriteTraces_OmitParentSpanIDReference(t *testing.T) {
	setOmitParentSpanIDReferenceGate(t, true)

	coreWriter := &mocks.Writer{}
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "testing-service")
	span := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op-1")
	span.SetParentSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	dbSpans := ToDBModel(td)
	require.Empty(t, dbSpans[0].References)
	writer := TraceWriter{spanWriter: coreWriter}
	coreWriter.On("WriteSpans", mock.Anything, dbSpans).Return(nil)
	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
}

// TestTraceWriter_WriteTraces_Error pins that WriteTraces returns the core
// writer's error verbatim, so a synchronous implementation can propagate real
// write failures to the caller.
func TestTraceWriter_WriteTraces_Error(t *testing.T) {
	coreWriter := &mocks.Writer{}
	td := ptrace.NewTraces()
	td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	writer := TraceWriter{spanWriter: coreWriter}
	wantErr := errors.New("bulk write failed")
	coreWriter.On("WriteSpans", mock.Anything, mock.Anything).Return(wantErr)
	err := writer.WriteTraces(context.Background(), td)
	require.ErrorIs(t, err, wantErr)
}

func TestTraceWriter_Close(t *testing.T) {
	coreWriter := &mocks.Writer{}
	coreWriter.On("Close").Return(nil)
	writer := TraceWriter{spanWriter: coreWriter}
	err := writer.Close()
	require.NoError(t, err)
}

func Test_NewTraceWriter(t *testing.T) {
	params := core.SpanWriterParams{
		Logger:         zap.NewNop(),
		MetricsFactory: metrics.NullFactory,
	}
	writer := NewTraceWriter(params)
	assert.NotNil(t, writer)
}
