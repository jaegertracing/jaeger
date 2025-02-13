// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type ClickhouseIntegrationTestSuite struct {
	StorageIntegration
	factory *clickhouse.Factory
}

func (s *ClickhouseIntegrationTestSuite) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func (s *ClickhouseIntegrationTestSuite) initialize(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	cfg := config.DefaultConfiguration()
	cfg.ConnConfig.Database = "jaeger"
	cfg.PoolConfig.ClientConfig.Database = "jaeger"
	f, err := clickhouse.NewFactory(&cfg, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})

	traceWriter, err := f.CreateTraceWriter()
	require.NoError(t, err)
	traceReader, err := f.CreateTracReader()
	require.NoError(t, err)

	s.TraceWriter = traceWriter
	s.TraceReader = traceReader
	s.factory = f
	s.CleanUp = s.cleanUp
}

func TestClickHouseStorage(t *testing.T) {
	SkipUnlessEnv(t, "clickhouse")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForClickhouse(t)
	})
	s := &ClickhouseIntegrationTestSuite{}
	s.initialize(t)
	s.testGetTrace(t)
}
