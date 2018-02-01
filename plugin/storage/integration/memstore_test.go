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

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

type MemStorageIntegrationTestSuite struct {
	StorageIntegration
}

func (s *MemStorageIntegrationTestSuite) initialize() error {
	logger, _ := testutils.NewLogger()
	s.logger = logger

	store := memory.NewStore()
	s.SpanReader = store
	s.SpanWriter = store

	// TODO DependencyWriter is not implemented in memory store

	s.Refresh = s.refresh
	s.CleanUp = s.cleanUp
	return nil
}

func (s *MemStorageIntegrationTestSuite) refresh() error {
	return nil
}

func (s *MemStorageIntegrationTestSuite) cleanUp() error {
	return s.initialize()
}

func TestMemoryStorage(t *testing.T) {
	s := &MemStorageIntegrationTestSuite{}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}
