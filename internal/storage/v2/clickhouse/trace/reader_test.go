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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type traceReaderTest struct {
	connection *mocks.Conn
	logger     *zap.Logger
	logBuffer  *testutils.Buffer
	reader     *Reader
}

func withTraceReader(fn func(r *traceReaderTest)) {
	conn := &mocks.Conn{}
	rows := &mocks.Rows{}
	rows.On("Err").Return(nil)
	rows.On("Scan", mock.Arguments{}).Return()
	conn.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(rows)
	logger, logBuffer := testutils.NewLogger()
	reader, _ := NewTraceReader(conn)
	r := &traceReaderTest{
		connection: conn,
		logger:     logger,
		logBuffer:  logBuffer,
		reader:     reader,
	}
	fn(r)
}

func TestNewTraceReader(t *testing.T) {
	t.Run("test trace reader creation", func(t *testing.T) {
		withTraceReader(func(r *traceReaderTest) {
			assert.NotNil(t, r.reader)
		})
	})
}

func TestTraceReader(t *testing.T) {
	t.Run("test Reader read successfully", func(t *testing.T) {
		withTraceReader(func(r *traceReaderTest) {
			seq := r.reader.GetTraces(context.Background())
			for traces, err := range seq {
				assert.NotNil(t, traces)
				assert.NoError(t, err)
			}
		})
	})
	t.Run("test Reader get empty traces", func(t *testing.T) {
		conn := &mocks.Conn{}
		rows := &mocks.Rows{}
		rows.On("Scan").Return(nil)
		rows.On("Err").Return(nil)
		conn.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).Return(rows)
		reader, _ := NewTraceReader(conn)
		seq := reader.GetTraces(context.Background())
		for traces, err := range seq {
			assert.Nil(t, traces)
			assert.NoError(t, err)
		}
	})
	t.Run("test Reader get error", func(t *testing.T) {
		conn := &mocks.Conn{}
		rows := &mocks.Rows{}

		rows.On("Scan", mock.Arguments{}).Return(nil)
		rows.On("Err").Return(errors.New("reade error"))
		conn.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(rows)
		reader, _ := NewTraceReader(conn)
		seq := reader.GetTraces(context.Background())
		assert.NotNil(t, seq)
		for traces, err := range seq {
			assert.Nil(t, traces)
			require.Error(t, err)
			assert.EqualError(t, err, "reade error")
		}
	})
}
