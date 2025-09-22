// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	v1badger "github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/badger"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	factory *badger.Factory
}

func (s *BadgerIntegrationStorage) initialize(t *testing.T) {
	cfg := v1badger.DefaultConfig()
	cfg.Ephemeral = false
	var err error
	s.factory, err = badger.NewFactory(*cfg, metrics.NullFactory, zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())))
	require.NoError(t, err)
	t.Cleanup(func() {
		s.factory.Close()
	})

	s.TraceWriter, err = s.factory.CreateTraceWriter()
	require.NoError(t, err)

	s.TraceReader, err = s.factory.CreateTraceReader()
	require.NoError(t, err)

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
