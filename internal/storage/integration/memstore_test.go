// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/v1adapter"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type MemStorageIntegrationTestSuite struct {
	StorageIntegration
	logger *zap.Logger
}

func (s *MemStorageIntegrationTestSuite) initialize(_ *testing.T) {
	s.logger, _ = testutils.NewLogger()

	store := memory.NewStore()
	s.SamplingStore = memory.NewSamplingStore(2)
	s.TraceReader = v1adapter.NewTraceReader(store)
	s.TraceWriter = v1adapter.NewTraceWriter(store)

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
