// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration

import (
	"testing"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
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
	s.SpanReader = store
	s.SpanWriter = store
	s.ArchiveSpanReader = archiveStore
	s.ArchiveSpanWriter = archiveStore

	// TODO DependencyWriter is not implemented in memory store

	s.CleanUp = s.initialize
}

func TestMemoryStorage(t *testing.T) {
	SkipUnlessEnv(t, "memory")
	s := &MemStorageIntegrationTestSuite{}
	s.initialize(t)
	s.RunAll(t)
}
