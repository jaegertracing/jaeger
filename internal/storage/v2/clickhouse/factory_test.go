// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type mockPoolBuilder struct {
	error error
}

func (m *mockPoolBuilder) NewPool(*config.Configuration, *zap.Logger) (client.Pool, error) {
	if m.error == nil {
		c := &mocks.Pool{}
		c.On("Do", context.Background(), mock.Anything, mock.Anything).Return(nil)
		c.On("Close").Return(nil)
		return c, nil
	}
	return nil, m.error
}

type mockConnBuilder struct {
	err error
}

func (m *mockConnBuilder) NewConn(*config.Configuration) (client.Conn, error) {
	if m.err == nil {
		c := &mocks.Conn{}
		c.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
		c.On("Exec", mock.Anything, mock.Anything).Return(nil)
		c.On("Close").Return(nil)
		return c, nil
	}
	return nil, m.err
}

func TestTraceFactory(t *testing.T) {
	var err error
	cfg := config.Configuration{}
	f := newFactory()

	poolBuilder := &mockPoolBuilder{}

	f.chPool, err = poolBuilder.NewPool(&cfg, zap.NewNop())
	require.NoError(t, err)

	connBuilder := &mockConnBuilder{}
	f.connection, err = connBuilder.NewConn(&cfg)
	require.NoError(t, err)

	_, err = f.CreateTraceWriter()
	require.NoError(t, err)
	_, err = f.CreateTracReader()
	require.NoError(t, err)

	err = f.Purge(context.Background())
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestPurge(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		conn := mocks.Conn{}
		conn.On("Exec", mock.Anything, mock.Anything).Return(nil)
		f := newFactory()
		f.connection = &conn
		err := f.Purge(context.Background())
		require.NoError(t, err)
	})
	t.Run("Connection refused", func(t *testing.T) {
		conn := mocks.Conn{}
		conn.On("Exec", mock.Anything, mock.Anything).Return(errors.New("connection refused"))
		f := newFactory()
		f.connection = &conn
		err := f.Purge(context.Background())
		require.Error(t, err, "connection refused")
	})
}

func TestClose(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		conn := mocks.Conn{}
		conn.On("Close").Return(nil)
		pool := mocks.Pool{}
		pool.On("Close").Return(nil)
		f := newFactory()
		f.connection = &conn
		f.chPool = &pool
		err := f.Close()
		require.NoError(t, err)
	})
	t.Run("Close failed", func(t *testing.T) {
		conn := mocks.Conn{}
		conn.On("Close").Return(nil)
		pool := mocks.Pool{}
		pool.On("Close").Return(errors.New("chPool close error"))
		f := newFactory()
		f.connection = &conn
		f.chPool = &pool
		require.Error(t, f.Close(), "chPool close error")

		conn.On("Close").Return(errors.New("clickhouse close error"))
		require.Error(t, f.Close(), "clickhouse close error")
	})
}

func TestCreateTraceWriter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		pool := mocks.Pool{}
		f := newFactory()
		f.chPool = &pool
		writer, err := f.CreateTraceWriter()
		require.NoError(t, err)
		require.NotNil(t, writer)
	})
	t.Run("Can't create trace writer with nil chPool", func(t *testing.T) {
		f := newFactory()
		f.chPool = nil
		writer, err := f.CreateTraceWriter()
		require.Error(t, err, "can't create trace writer with nil chPool")
		require.Empty(t, writer)
	})
}

func TestCreateTraceReader(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		c := mocks.Conn{}
		f := newFactory()
		f.connection = &c
		reader, err := f.CreateTracReader()
		require.NoError(t, err)
		require.NotNil(t, reader)
	})
	t.Run("Can't create trace reader with nil clickhouse", func(t *testing.T) {
		f := newFactory()
		f.connection = nil
		writer, err := f.CreateTracReader()
		require.Error(t, err, "can't create trace reader with nil clickhouse")
		require.Nil(t, writer)
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
