// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type MemStorageIntegrationTestSuite struct {
	StorageIntegration
	logger *zap.Logger
}

func (s *MemStorageIntegrationTestSuite) initialize(_ *testing.T) {
	s.logger, _ = testutils.NewLogger()

	store := memory.NewStore()
	archiveStore := memory.NewStore()
	s.SamplingStore = memory.NewSamplingStore(2)
	spanReader := store
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.SpanWriter = store
	s.ArchiveSpanReader = archiveStore
	s.ArchiveSpanWriter = archiveStore

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
