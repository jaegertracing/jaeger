// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	storemocks "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var traceId = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

func TestErrNewTraceWriter(t *testing.T) {
	session := getSessionWithError(errors.New("test error"))
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

func TestWriteTracesWithError(t *testing.T) {
	td := ptrace.NewTraces()
	td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("operation-a")
	td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("operation-b")
	mockWriter := &storemocks.CoreSpanWriter{}
	expectedErr := errors.New("test error")
	mockWriter.On("WriteSpan", mock.Anything).Return(expectedErr)
	writer := TraceWriter{writer: mockWriter}
	err := writer.WriteTraces(context.Background(), td)
	expected := "test error\ntest error"
	require.ErrorContains(t, err, expected)
}

func TestTraceWriterClose(t *testing.T) {
	mockWriter := &storemocks.CoreSpanWriter{}
	mockWriter.On("Close").Return(nil)
	writer := TraceWriter{writer: mockWriter}
	require.NoError(t, writer.Close())
}

func getSessionWithError(err error) *mocks.Session {
	tableCheckStmt := "SELECT * from %s limit 1"
	session := &mocks.Session{}
	query := &mocks.Query{}
	query.On("Exec").Return(err)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, "operation_names"),
		mock.Anything).Return(query)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, "operation_names_v2"),
		mock.Anything).Return(query)
	return session
}
