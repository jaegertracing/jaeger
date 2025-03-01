// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/conn"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/pool"
)

type Configuration struct {
	ConnConfig   conn.Configuration
	PoolConfig   pool.Configuration
	CreateSchema bool
}

func DefaultConfiguration() Configuration {
	return Configuration{
		conn.DefaultConfig(),
		pool.DefaultConfig(),
		false,
	}
}
