// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

type BadgerStorageIntegration struct {
	E2EStorageIntegration
	logger  *zap.Logger
	factory *badger.Factory
}

func (s *BadgerStorageIntegration) initialize(t *testing.T) {
	s.factory = badger.NewFactory()

	err := s.factory.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)

	s.Refresh = func(_ *testing.T) {}
	s.CleanUp = s.cleanUp

	s.logger, _ = testutils.NewLogger()

	// TODO: remove this badger supports returning spanKind from GetOperations
	s.GetOperationsMissingSpanKind = true
	s.SkipArchiveTest = true
}

func (s *BadgerStorageIntegration) Close() error {
	return s.factory.Close()
}

func (s *BadgerStorageIntegration) cleanUp(t *testing.T) {
	err := s.Close()
	require.NoError(t, err)
	s.initialize(t)
}

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &BadgerStorageIntegration{
	    ConfigFile: "cmd/jaeger/badger_config.yaml"
	    SkipBinaryAttrs: true,
   }

	s.initialize(t)
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
		s.Close()
	})
	s.RunAll(t)
}
