// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	v1badger "github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/badger"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type BadgerIntegrationStorage struct {
	StorageIntegration
	factory *badger.Factory
}

func (s *BadgerIntegrationStorage) initialize() error {
	cfg := v1badger.DefaultConfig()
	cfg.Ephemeral = false
	var err error
	s.factory, err = badger.NewFactory(
		*cfg,
		metrics.NullFactory,
		zap.NewNop(), // logger for non-test usage
	)
	if err != nil {
		return err
	}
	s.TraceWriter, err = s.factory.CreateTraceWriter()
	if err != nil {
		return err
	}
	s.TraceReader, err = s.factory.CreateTraceReader()
	if err != nil {
		return err
	}
	s.SamplingStore, err = s.factory.CreateSamplingStore(0)
	if err != nil {
		return err
	}
	return nil
}

func (s *BadgerIntegrationStorage) cleanUp() error {
	return s.factory.Purge(context.Background())
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
	t.Cleanup(func() {
		require.NoError(t, s.cleanUp())
	})
	require.NoError(t, s.initialize())

	s.RunAll(t)
}
