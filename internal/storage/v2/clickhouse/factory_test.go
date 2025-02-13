// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/clickhouse/config"
)

func TestClickHouseFactoryWithConfig(t *testing.T) {
	cfg := config.Configuration{
		ClientConfig: config.ClientConfig{
			Address:  "127.0.0.1:9000",
			Database: "jaeger",
			Username: "default",
			Password: "default",
		},
		ConnectionPoolConfig: config.ConnectionPoolConfig{},
	}

	_, err := NewFactoryWithConfig(&cfg, zap.NewNop())
	require.NoError(t, err)
}
