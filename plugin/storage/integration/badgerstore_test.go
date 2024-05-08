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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	factory *badger.Factory
}

func (s *BadgerIntegrationStorage) initialize(t *testing.T) {
	s.factory = badger.NewFactory()
	s.factory.Options.Primary.Ephemeral = false

	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	err := s.factory.Initialize(metrics.NullFactory, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		s.factory.Close()
	})

	s.SpanWriter, err = s.factory.CreateSpanWriter()
	require.NoError(t, err)

	s.SpanReader, err = s.factory.CreateSpanReader()
	require.NoError(t, err)

	s.SamplingStore, err = s.factory.CreateSamplingStore(0)
	require.NoError(t, err)
}

func (s *BadgerIntegrationStorage) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func TestBadgerStorage(t *testing.T) {
	SkipUnlessEnv(t, "badger")
	s := &BadgerIntegrationStorage{
		StorageIntegration: StorageIntegration{
			SkipArchiveTest: true,

			// TODO: remove this badger supports returning spanKind from GetOperations
			GetOperationsMissingSpanKind: true,
		},
	}
	s.CleanUp = s.cleanUp
	s.initialize(t)
	s.RunAll(t)
}
