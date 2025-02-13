// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/ClickHouse/ch-go/cht"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
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
		cht.WithLog(logger),
	)

	client, err := cfg.NewClient(logger)
	require.NoError(t, err)
	assert.NotEmpty(t, client)
	defer client.Close()
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
