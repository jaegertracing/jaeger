// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/pkg/clickhouse/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type ClickhouseIntegrationTestSuite struct {
	StorageIntegration
}

func (s *ClickhouseIntegrationTestSuite) initialize(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))

	cfg := config.DefaultConfiguration()
	f, err := clickhouse.NewFactoryWithConfig(&cfg, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})

	traceWriter, err := f.CreateTraceWriter()
	require.NoError(t, err)
	s.TraceWriter = traceWriter
	s.CleanUp = func(_ *testing.T) {}
}

func TestClickHouseStorage(t *testing.T) {
	SkipUnlessEnv(t, "clickhouse")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &ClickhouseIntegrationTestSuite{}
	s.initialize(t)
}
