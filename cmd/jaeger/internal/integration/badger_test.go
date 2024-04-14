// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

type BadgerStorageIntegration struct {
	E2EStorageIntegration
	logger *zap.Logger
}

func (s *BadgerStorageIntegration) initialize(t *testing.T) {
	s.e2eInitialize(t)

	s.CleanUp = s.cleanUp
	s.logger = zap.NewNop()
}

func (s *BadgerStorageIntegration) cleanUp(t *testing.T) {
	s.e2eCleanUp(t)
	s.initialize(t)
}

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &BadgerStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "cmd/jaeger/badger_config.yaml",
			StorageIntegration: integration.StorageIntegration{
				SkipBinaryAttrs: true,
				SkipArchiveTest: true,

				// TODO: remove this once badger supports returning spanKind from GetOperations
				// Cf https://github.com/jaegertracing/jaeger/issues/1922
				GetOperationsMissingSpanKind: true,
			},
		},
	}

	s.initialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunAll(t)
}
