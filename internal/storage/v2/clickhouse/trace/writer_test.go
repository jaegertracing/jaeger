// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

var sampleTrace = ptrace.NewTraces()

type traceWriterTest struct {
	pool      *mocks.Pool
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *Writer
}

func withTraceWriter(fn func(w *traceWriterTest)) {
	pool := &mocks.Pool{}
	pool.On("Do", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	logger, logBuffer := testutils.NewLogger()
	writer, _ := NewTraceWriter(pool, logger)
	w := &traceWriterTest{
		pool:      pool,
		logger:    logger,
		logBuffer: logBuffer,
		writer:    writer,
	}
	fn(w)
}

func TestNewTraceWriter(t *testing.T) {
	t.Run("test trace writer creation", func(t *testing.T) {
		withTraceWriter(func(w *traceWriterTest) {
			assert.NotNil(t, w.writer)
		})
	})
}

func TestTraceWriter(t *testing.T) {
	t.Run("test TraceWriter write successfully", func(t *testing.T) {
		withTraceWriter(func(w *traceWriterTest) {
			err := w.writer.WriteTraces(context.Background(), sampleTrace)
			require.NoError(t, err)
		})
	})
	t.Run("test TraceWriter write error", func(t *testing.T) {
		pool := &mocks.Pool{}
		pool.On("Do", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("database/table don't exist"))
		logger, _ := testutils.NewLogger()
		writer, err := NewTraceWriter(pool, logger)
		require.NoError(t, err)
		w := &traceWriterTest{
			writer: writer,
		}
		err = w.writer.WriteTraces(context.Background(), sampleTrace)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database/table don't exist")
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
