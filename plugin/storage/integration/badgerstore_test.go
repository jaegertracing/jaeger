// Copyright (c) 2018 The Jaeger Authors.
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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	logger  *zap.Logger
	factory *badger.Factory
}

func (s *BadgerIntegrationStorage) initialize(t *testing.T) {
	s.factory = badger.NewFactory()

	err := s.factory.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)

	s.SpanWriter, err = s.factory.CreateSpanWriter()
	require.NoError(t, err)

	s.SpanReader, err = s.factory.CreateSpanReader()
	require.NoError(t, err)

	s.SamplingStore, err = s.factory.CreateSamplingStore(0)
	require.NoError(t, err)

	s.CleanUp = s.cleanUp

	s.logger, _ = testutils.NewLogger()

	// TODO: remove this badger supports returning spanKind from GetOperations
	s.GetOperationsMissingSpanKind = true
	s.SkipArchiveTest = true
}

func (s *BadgerIntegrationStorage) clear() error {
	return s.factory.Close()
}

func (s *BadgerIntegrationStorage) cleanUp(t *testing.T) {
	err := s.clear()
	require.NoError(t, err)
	s.initialize(t)
}

func TestBadgerStorage(t *testing.T) {
	SkipUnlessEnv(t, "badger")
	s := &BadgerIntegrationStorage{}
	s.initialize(t)
	s.RunAll(t)
	defer s.clear()
}
