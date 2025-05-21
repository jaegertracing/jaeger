// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
)

func TestFactory(t *testing.T) {
	f := NewFactory(config.DefaultConfiguration())
	require.NotNil(t, f)

	mockConn := new(mocks.Conn)
	mockConn.On("Close").Return(nil)

	f.newConnFn = func(context.Context, config.Config) (client.Conn, error) {
		return mockConn, nil
	}
	require.NotNil(t, f)

	err := f.Start(context.Background())
	require.NoError(t, err)

	reader, err := f.CreateTraceReader()
	require.NoError(t, err)
	require.NotNil(t, reader)

	writer, err := f.CreateTraceWriter()
	require.NoError(t, err)
	require.NotNil(t, writer)

	err = f.Close()
	require.NoError(t, err)
}

func TestNewFactory(t *testing.T) {
	f := NewFactory(config.DefaultConfiguration())
	require.NotNil(t, f)
	require.NotNil(t, f.newConnFn)
}

func TestCreateTraceReader(t *testing.T) {
	f := Factory{}
	f.conn = &mocks.Conn{}
	reader, err := f.CreateTraceReader()
	require.NoError(t, err)
	require.NotNil(t, reader)
}

func TestCreateTraceWriter(t *testing.T) {
	f := Factory{}
	f.conn = &mocks.Conn{}
	writer, err := f.CreateTraceWriter()
	require.NoError(t, err)
	require.NotNil(t, writer)
}

func TestFactory_Start(t *testing.T) {
	f := Factory{}
	f.cfg = config.DefaultConfiguration()
	ctx := context.Background()

	mockConn := new(mocks.Conn)

	t.Run("Successful", func(t *testing.T) {
		connFn := func(context.Context, config.Config) (client.Conn, error) {
			return mockConn, nil
		}

		f.newConnFn = connFn
		require.NoError(t, f.Start(ctx))
	})

	t.Run("Get connection failed", func(t *testing.T) {
		connFn := func(context.Context, config.Config) (client.Conn, error) {
			return nil, fmt.Errorf("connect to servers %s time out", f.cfg.Servers)
		}

		f.newConnFn = connFn

		require.EqualError(t, f.Start(ctx), fmt.Sprintf("connect to servers %s time out", f.cfg.Servers))
	})
}

func TestFactory_Close(t *testing.T) {
	t.Run("successfully", func(t *testing.T) {
		mockConn := new(mocks.Conn)
		factory := &Factory{
			cfg:  config.DefaultConfiguration(),
			conn: mockConn,
		}
		mockConn.On("Close").Return(nil)

		require.NoError(t, factory.Close())
	})
	t.Run("close connection failed", func(t *testing.T) {
		mockConn := new(mocks.Conn)
		factory := &Factory{
			cfg:  config.DefaultConfiguration(),
			conn: mockConn,
		}
		expectedErr := fmt.Errorf("failed to close connections with remote servers:%s", factory.cfg.Servers)
		mockConn.On("Close").Return(expectedErr)
		err := factory.Close()
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to close connections with remote servers")
	})
	t.Run("pool and connection not exist", func(t *testing.T) {
		factory := &Factory{
			cfg:  config.DefaultConfiguration(),
			conn: nil,
		}
		require.NoError(t, factory.Close())
	})
}
