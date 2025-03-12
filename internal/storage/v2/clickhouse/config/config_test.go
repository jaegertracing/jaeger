// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/conn"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/pool"
)

func TestDefaultConfig(t *testing.T) {
	excepted := Configuration{
		ConnConfig:   conn.DefaultConfig(),
		PoolConfig:   pool.DefaultConfig(),
		CreateSchema: true,
	}

	actual := DefaultConfiguration()
	assert.Equal(t, excepted, actual)
}
