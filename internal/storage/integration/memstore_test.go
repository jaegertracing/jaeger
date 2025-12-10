// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type MemStorageIntegrationTestSuite struct {
	StorageIntegration
	logger *zap.Logger
}

func (s *MemStorageIntegrationTestSuite) initialize(t *testing.T) {
	s.logger, _ = testutils.NewLogger()
	telset := telemetry.NoopSettings()
	telset.Logger = s.logger

	f, err := memory.NewFactory(memory.Configuration{MaxTraces: 10000}, telset)
	require.NoError(t, err)
	traceReader, err := f.CreateTraceReader()
	require.NoError(t, err)
	traceWriter, err := f.CreateTraceWriter()
	require.NoError(t, err)

	s.SamplingStore = memory.NewSamplingStore(2)
	s.TraceReader = traceReader
	s.TraceWriter = traceWriter

	// TODO DependencyWriter is not implemented in memory store

	s.CleanUp = s.initialize
}

func TestMemoryStorage(t *testing.T) {
	SkipUnlessEnv(t, "memory")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &MemStorageIntegrationTestSuite{}
	s.initialize(t)
	s.RunAll(t)
}
