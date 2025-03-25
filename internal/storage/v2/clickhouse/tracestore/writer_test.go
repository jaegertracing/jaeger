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
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var sampleTrace = ptrace.NewTraces()

type traceWriterTest struct {
	pool      *mocks.ChPool
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *TraceWriter
}

func withTraceWriter(fn func(w *traceWriterTest)) {
	pool := &mocks.ChPool{}
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
	t.Run("should create trace writer successfully", func(t *testing.T) {
		withTraceWriter(func(w *traceWriterTest) {
			assert.NotNil(t, w.writer)
		})
	})
	t.Run("should fail to create trace writer when ClickHouse connection pool is nil", func(t *testing.T) {
		writer, err := NewTraceWriter(nil, zap.NewNop())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't create trace writer with nil chPool")
		assert.Nil(t, writer)
	})
}

func TestTraceWriter(t *testing.T) {
	t.Run("should write traces successfully", func(t *testing.T) {
		withTraceWriter(func(w *traceWriterTest) {
			err := w.writer.WriteTraces(context.Background(), sampleTrace)
			require.NoError(t, err)
		})
	})
	t.Run("should return error when writing traces fails due to database/table issues", func(t *testing.T) {
		pool := &mocks.ChPool{}
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
