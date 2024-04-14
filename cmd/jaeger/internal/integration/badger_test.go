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

	s.Refresh = func(_ *testing.T) {}
	s.CleanUp = s.cleanUp

	s.logger = zap.NewNop()

	// TODO: remove this once badger supports returning spanKind from GetOperations
	s.GetOperationsMissingSpanKind = true
	s.SkipArchiveTest = true
}

func (s *BadgerStorageIntegration) cleanUp(t *testing.T) {
	s.e2eCleanUp(t)
	s.initialize(t)
}

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &BadgerStorageIntegration{}
	s.ConfigFile = "cmd/jaeger/badger_config.yaml"
	s.SkipBinaryAttrs = true

	s.initialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunAll(t)
}
