// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
	t.Helper()
	s.factory = badger.NewFactory()
	s.factory.Config.Ephemeral = false

	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
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
	t.Helper()
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
