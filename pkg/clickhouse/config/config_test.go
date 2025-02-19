// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/ClickHouse/ch-go/cht"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfiguration()
	assert.NotEmpty(t, cfg)
	assert.NotEmpty(t, cfg.ClientConfig)
	assert.NotEmpty(t, cfg.ConnectionPoolConfig)
}

func TestNewClientWithDefaults(t *testing.T) {
	cfg := DefaultConfiguration()
	logger := zap.NewNop()

	cht.New(t,
		cht.WithLog(zap.NewNop()),
	)

	client, err := cfg.NewClient(logger)

	require.NoError(t, err)
	assert.NotEmpty(t, client)
	defer client.Close()
}

func TestNewPool(t *testing.T) {
	cfg := DefaultConfiguration()
	logger := zap.NewNop()
	cht.New(t,
		cht.WithLog(zap.NewNop()),
	)
	conn, err := cfg.newPool(logger)

	require.NoError(t, err)
	assert.NotEmpty(t, conn)
	defer conn.Close()
}

func TestNewPoolFail(t *testing.T) {
	cfg := Configuration{}
	logger := zap.NewNop()
	cht.New(t,
		cht.WithLog(zap.NewNop()),
	)
	pool, err := cfg.newPool(logger)

	require.Error(t, err)
	assert.Nil(t, pool)
}

func TestNewConnection(t *testing.T) {
	cfg := DefaultConfiguration()
	cht.New(t,
		cht.WithLog(zap.NewNop()),
	)
	conn, err := cfg.newConn()
	require.NoError(t, err)
	assert.NotEmpty(t, conn)
	defer conn.Close()
}
