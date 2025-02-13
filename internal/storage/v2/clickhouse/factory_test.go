// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"

	"github.com/ClickHouse/ch-go/cht"
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
	// provide a tcp server for testing.
	cht.New(t,
		cht.WithLog(zap.NewNop()),
	)
	_, err := NewFactoryWithConfig(&cfg, zap.NewNop())
	require.NoError(t, err)
}

func TestCreateTraceWriter(t *testing.T) {
	cfg := config.DefaultConfiguration()
	// provide a tcp server for testing.
	cht.New(t,
		cht.WithLog(zap.NewNop()),
	)

	f, err := NewFactoryWithConfig(&cfg, zap.NewNop())
	require.NoError(t, err)
	_, err = f.CreateTraceWriter()
	require.NoError(t, err)
}
