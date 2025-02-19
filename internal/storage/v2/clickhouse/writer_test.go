// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

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
	conventions "go.opentelemetry.io/collector/semconv/v1.27.0"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/clickhouse/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type traceWriterTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *TraceWriter
}

func withTraceWriter(t *testing.T, fn func(w *traceWriterTest)) {
	client := &mocks.Client{}
	client.On("Do").Return(nil)
	logger, logBuffer := testutils.NewLogger()
	writer, err := NewTraceWriter(client, logger, "otel_traces")
	require.NoError(t, err)
	w := &traceWriterTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		writer:    writer,
	}
	fn(w)
}

func TestNewTraceWriter(t *testing.T) {
	t.Run("test trace writer creation", func(t *testing.T) {
		withTraceWriter(t, func(w *traceWriterTest) {
			assert.NotNil(t, w.writer)
		})
	})
}

func TestTraceWriter_WriteTrace(t *testing.T) {
	testCases := []struct {
		caption       string
		expectedError string
		expectedLogs  []string
	}{
		{
			caption:       "traces insertion successfully",
			expectedError: "",
		},
		{
			caption:       "traces insertion failed",
			expectedError: "table not exists",
		},
	}
	traces := simpleTraces(10)
	for _, tc := range testCases {
		testCases := tc
		t.Run(tc.caption, func(t *testing.T) {
			withTraceWriter(t, func(w *traceWriterTest) {
				if testCases.expectedError == "" {
					w.client.On("Do", mock.Anything, mock.Anything).Return(nil)
				} else {
					w.client.On("Do", mock.Anything, mock.Anything).Return(errors.New(testCases.expectedError))
				}
				err := w.writer.WriteTraces(context.Background(), traces)

				if testCases.expectedError == "" {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, testCases.expectedError)
				}
				for _, expectedLog := range testCases.expectedLogs {
					assert.Contains(t, w.logBuffer.String(), expectedLog)
				}
				if len(testCases.expectedLogs) == 0 {
					assert.Equal(t, "", w.logBuffer.String())
				}
			})
		})
	}
}

func simpleTraces(count int) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://opentelemetry.io/schemas/1.4.0")
	rs.Resource().SetDroppedAttributesCount(10)
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("io.opentelemetry.contrib.clickhouse")
	ss.Scope().SetVersion("1.0.0")
	ss.SetSchemaUrl("https://opentelemetry.io/schemas/1.7.0")
	ss.Scope().SetDroppedAttributesCount(20)
	ss.Scope().Attributes().PutStr("lib", "clickhouse")
	timestamp := time.Unix(1703498029, 0)
	for i := 0; i < count; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID([16]byte{1, 2, 3, byte(i)})
		s.SetSpanID([8]byte{1, 2, 3, byte(i)})
		s.TraceState().FromRaw("trace state")
		s.SetParentSpanID([8]byte{1, 2, 4, byte(i)})
		s.SetName("call db")
		s.SetKind(ptrace.SpanKindInternal)
		s.SetStartTimestamp(pcommon.NewTimestampFromTime(timestamp))
		s.SetEndTimestamp(pcommon.NewTimestampFromTime(timestamp.Add(time.Minute)))
		s.Attributes().PutStr(conventions.AttributeServiceName, "v")
		s.Status().SetMessage("error")
		s.Status().SetCode(ptrace.StatusCodeError)
		event := s.Events().AppendEmpty()
		event.SetName("event1")
		event.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
		event.Attributes().PutStr("level", "info")
		link := s.Links().AppendEmpty()
		link.SetTraceID([16]byte{1, 2, 5, byte(i)})
		link.SetSpanID([8]byte{1, 2, 5, byte(i)})
		link.TraceState().FromRaw("error")
		link.Attributes().PutStr("k", "v")
	}
	return traces
}
