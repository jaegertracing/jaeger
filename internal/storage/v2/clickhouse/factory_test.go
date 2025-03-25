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
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type mockPoolBuilder struct {
	error error
}

func (m *mockPoolBuilder) NewPool(*config.Configuration, *zap.Logger) (client.ChPool, error) {
	if m.error == nil {
		c := &mocks.ChPool{}
		c.On("Do", context.Background(), mock.Anything, mock.Anything).Return(nil)
		c.On("Close").Return(nil)
		return c, nil
	}
	return nil, m.error
}

type mockConnBuilder struct {
	err error
}

func (m *mockConnBuilder) NewConn(*config.Configuration) (client.Clickhouse, error) {
	if m.err == nil {
		c := &mocks.Clickhouse{}
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
	f.ClickhouseClient, err = connBuilder.NewConn(&cfg)
	require.NoError(t, err)

	_, err = f.CreateTraceWriter()
	require.NoError(t, err)
	_, err = f.CreateTracReader()
	require.NoError(t, err)

	err = f.Purge(context.Background())
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestNewClientPrerequisites(t *testing.T) {
	t.Run("should not create schema when CreateSchema is false", func(t *testing.T) {
		cfg := config.DefaultConfiguration()
		cfg.CreateSchema = false
		err := newClientPrerequisites(&cfg, zap.NewNop())
		require.NoError(t, err)
	})
}

func TestPurge(t *testing.T) {
	tests := []struct {
		name         string
		mockError    error
		expectError  bool
		errorMessage string
	}{
		{
			name:        "should succeed when Exec does not return an error",
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "should return error when ClickhouseClient is refused",
			mockError:    errors.New("ClickhouseClient refused"),
			expectError:  true,
			errorMessage: "ClickhouseClient refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := mocks.Clickhouse{}
			conn.On("Exec", mock.Anything, mock.Anything).Return(tt.mockError)
			f := newFactory()
			f.ClickhouseClient = &conn
			err := f.Purge(context.Background())

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClose(t *testing.T) {
	tests := []struct {
		name         string
		poolCloseErr error
		connCloseErr error
		expectError  bool
		errorMessage string
	}{
		{
			name:         "should succeed when both ClickhouseClient and pool close without error",
			poolCloseErr: nil,
			connCloseErr: nil,
			expectError:  false,
		},
		{
			name:         "should return error if chPool Close fails",
			poolCloseErr: errors.New("chPool close error"),
			connCloseErr: nil,
			expectError:  true,
			errorMessage: "chPool close error",
		},
		{
			name:         "should return error if clickhouse ClickhouseClient Close fails",
			poolCloseErr: nil,
			connCloseErr: errors.New("clickhouse close error"),
			expectError:  true,
			errorMessage: "clickhouse close error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := mocks.Clickhouse{}
			conn.On("Close").Return(tt.connCloseErr)

			pool := mocks.ChPool{}
			pool.On("Close").Return(tt.poolCloseErr)

			f := newFactory()
			f.ClickhouseClient = &conn
			f.chPool = &pool

			err := f.Close()

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateTraceWriter(t *testing.T) {
	t.Run("should succeed when trace writer is created successfully", func(t *testing.T) {
		pool := mocks.ChPool{}
		f := newFactory()
		f.chPool = &pool
		writer, err := f.CreateTraceWriter()
		require.NoError(t, err)
		require.NotNil(t, writer)
	})

	t.Run("should return error when chPool is nil", func(t *testing.T) {
		f := newFactory()
		f.chPool = nil
		writer, err := f.CreateTraceWriter()
		require.Error(t, err)
		require.Empty(t, writer)
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
