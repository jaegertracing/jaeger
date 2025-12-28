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

func (s *MemStorageIntegrationTestSuite) initialize() error {
	logger, _ := testutils.NewLogger()
	s.logger = logger

	telset := telemetry.NoopSettings()
	telset.Logger = s.logger

	f, err := memory.NewFactory(
		memory.Configuration{MaxTraces: 10000},
		telset,
	)
	if err != nil {
		return err
	}

	s.TraceReader, err = f.CreateTraceReader()
	if err != nil {
		return err
	}

	s.TraceWriter, err = f.CreateTraceWriter()
	if err != nil {
		return err
	}

	s.SamplingStore = memory.NewSamplingStore(2)

	// TODO DependencyWriter is not implemented in memory store

	return nil
}

func TestMemoryStorage(t *testing.T) {
	SkipUnlessEnv(t, "memory")

	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})

	s := &MemStorageIntegrationTestSuite{}

	require.NoError(t, s.initialize())

	// REQUIRED by StorageIntegration contract, even if no-op
	s.CleanUp = func(t *testing.T) {
		// no-op
	}

	s.RunAll(t)
}
