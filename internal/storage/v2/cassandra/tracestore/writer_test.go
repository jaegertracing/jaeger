// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	storemocks "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var traceId = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

func TestErrNewTraceWriter(t *testing.T) {
	session := spanstore.GetSessionWithError(errors.New("test error"))
	metricsFactory := metricstest.NewFactory(0)
	logger, _ := testutils.NewLogger()
	_, err := NewTraceWriter(session, 0, metricsFactory, logger)
	require.ErrorContains(t, err, "neither table operation_names_v2 nor operation_names exist")
}

func TestWriteTraces(t *testing.T) {
	td := ptrace.NewTraces()
	resourceSpan := td.ResourceSpans().AppendEmpty()
	resourceSpan.Resource().Attributes().PutStr("service.name", "service-a")
	span := resourceSpan.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceId)
	span.SetName("operation-a")
	span.Attributes().PutStr("x", "y")
	mockWriter := &storemocks.CoreSpanWriter{}
	dbSpan := ToDBModel(td)[0]
	mockWriter.On("WriteSpan", &dbSpan).Return(nil)
	writer := TraceWriter{writer: mockWriter}
	require.NoError(t, writer.WriteTraces(context.Background(), td))
}

func TestTraceWriterClose(t *testing.T) {
	mockWriter := &storemocks.CoreSpanWriter{}
	mockWriter.On("Close").Return(nil)
	writer := TraceWriter{writer: mockWriter}
	require.NoError(t, writer.Close())
}
