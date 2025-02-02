// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	factory *badger.Factory
}

func (s *BadgerIntegrationStorage) initialize(t *testing.T) {
	s.factory = badger.NewFactory()
	s.factory.Config.Ephemeral = false

	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	err := s.factory.Initialize(metrics.NullFactory, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		s.factory.Close()
	})

	spanWriter, err := s.factory.CreateSpanWriter()
	require.NoError(t, err)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)

	spanReader, err := s.factory.CreateSpanReader()
	require.NoError(t, err)
	s.TraceReader = v1adapter.NewTraceReader(spanReader)

	s.SamplingStore, err = s.factory.CreateSamplingStore(0)
	require.NoError(t, err)
}

func (s *BadgerIntegrationStorage) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func TestBadgerStorage(t *testing.T) {
	SkipUnlessEnv(t, "badger")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &BadgerIntegrationStorage{
		StorageIntegration: StorageIntegration{
			// TODO: remove this badger supports returning spanKind from GetOperations
			GetOperationsMissingSpanKind: true,
		},
	}
	s.CleanUp = s.cleanUp
	s.initialize(t)
	s.RunAll(t)
}
