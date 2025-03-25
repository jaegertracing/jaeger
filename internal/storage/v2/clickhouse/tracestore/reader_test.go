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

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type traceReaderTest struct {
	connection *mocks.Clickhouse
	logger     *zap.Logger
	logBuffer  *testutils.Buffer
	reader     *TraceReader
}

func withTraceReader(fn func(r *traceReaderTest)) {
	conn := &mocks.Clickhouse{}
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
	t.Run("should create trace reader successfully", func(t *testing.T) {
		withTraceReader(func(r *traceReaderTest) {
			assert.NotNil(t, r.reader)
		})
	})
	t.Run("should fail to create trace reader when ClickHouse client is nil", func(t *testing.T) {
		reader, err := NewTraceReader(nil)
		assert.Nil(t, reader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't create trace reader with nil clickhouse client")
	})
}

func TestGetTraces(t *testing.T) {
	t.Run("should read traces successfully", func(t *testing.T) {
		withTraceReader(func(r *traceReaderTest) {
			seq := r.reader.GetTraces(context.Background())
			for traces, err := range seq {
				assert.NotNil(t, traces)
				assert.NoError(t, err)
			}
		})
	})
	t.Run("should return empty traces when no data is available", func(t *testing.T) {
		conn := &mocks.Clickhouse{}
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

	t.Run("should return error when rows encounter an error", func(t *testing.T) {
		conn := &mocks.Clickhouse{}
		rows := &mocks.Rows{}

		rows.On("Scan", mock.Arguments{}).Return(nil)
		rows.On("Err").Return(errors.New("trace not found"))
		conn.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(rows)
		reader, err := NewTraceReader(conn)
		assert.NotNil(t, reader)
		require.NoError(t, err)
		seq := reader.GetTraces(context.Background())
		assert.NotNil(t, seq)
		for traces, err := range seq {
			assert.Nil(t, traces)
			require.Error(t, err)
			assert.EqualError(t, err, "trace not found")
		}
	})

	t.Run("should return error when querying ClickHouse fails", func(t *testing.T) {
		conn := &mocks.Clickhouse{}
		conn.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(nil,
			errors.New("can't connect to clickhouse"))
		reader, err := NewTraceReader(conn)
		assert.NotNil(t, reader)
		require.NoError(t, err)

		traces, err := reader.getTraces(context.Background(), "", tracestore.GetTraceParams{})
		assert.Nil(t, traces)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't connect to clickhouse")
	})

	t.Run("should return error when scanning rows fails", func(t *testing.T) {
		conn := &mocks.Clickhouse{}
		rows := &mocks.Rows{}

		conn.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(rows, nil)
		rows.On("Next").Return(true)
		rows.On("Scan",
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything).Return(errors.New("failed to scan"))
		reader, err := NewTraceReader(conn)
		assert.NotNil(t, reader)
		require.NoError(t, err)

		traces, err := reader.getTraces(context.Background(), "", tracestore.GetTraceParams{TraceID: pcommon.TraceID{byte(0), byte(2)}})
		assert.Equal(t, []ptrace.Traces{}, traces)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to scan")
	})

	t.Run("should return error when trace conversion fails", func(t *testing.T) {
		conn := &mocks.Clickhouse{}
		rows := &mocks.Rows{}

		conn.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(rows, nil)
		rows.On("Next").Return(true)
		rows.On("Scan",
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything).Return(nil)
		reader, err := NewTraceReader(conn)
		assert.NotNil(t, reader)
		require.NoError(t, err)
		traces, err := reader.getTraces(context.Background(), "", tracestore.GetTraceParams{TraceID: pcommon.TraceID{byte(0), byte(2)}})
		assert.Equal(t, []ptrace.Traces{}, traces)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trace id is empty")
	})
}

func TestGetServices(t *testing.T) {
	conn := &mocks.Clickhouse{}

	reader, _ := NewTraceReader(conn)
	servers, err := reader.GetServices(context.Background())
	assert.Equal(t, []string{}, servers)
	require.NoError(t, err)
}

func TestGetOperations(t *testing.T) {
	conn := &mocks.Clickhouse{}

	reader, _ := NewTraceReader(conn)
	operations, err := reader.GetOperations(context.Background(), tracestore.OperationQueryParams{})
	assert.Equal(t, []tracestore.Operation{}, operations)
	require.NoError(t, err)
}

func TestFindTraces(t *testing.T) {
	conn := &mocks.Clickhouse{}

	reader, _ := NewTraceReader(conn)
	seq := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{})

	var results [][]ptrace.Traces
	var errs []error

	seq(func(traces []ptrace.Traces, err error) bool {
		results = append(results, traces)
		errs = append(errs, err)
		return true
	})
	require.NoError(t, errs[0])
	assert.Empty(t, results[0])
}

func TestFindTraceIDs(t *testing.T) {
	conn := &mocks.Clickhouse{}

	reader, _ := NewTraceReader(conn)
	seq := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{})
	var results [][]tracestore.FoundTraceID
	var errs []error

	seq(func(ids []tracestore.FoundTraceID, err error) bool {
		results = append(results, ids)
		errs = append(errs, err)
		return true
	})
	require.NoError(t, errs[0])
	assert.Empty(t, results[0])
}
