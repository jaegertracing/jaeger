// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

type GRPCStorageIntegration struct {
	E2EStorageIntegration

	remoteStorage *integration.RemoteMemoryStorage
}

func (s *GRPCStorageIntegration) initialize(t *testing.T) {
	logger, _ := testutils.NewLogger()

	s.remoteStorage = integration.StartNewRemoteMemoryStorage(t, logger)

	s.CleanUp = s.cleanUp
}

func (s *GRPCStorageIntegration) cleanUp(t *testing.T) {
	s.remoteStorage.Close(t)
	s.initialize(t)
}

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	s := &GRPCStorageIntegration{}
	s.ConfigFile = "cmd/jaeger/grpc_config.yaml"
	s.SkipBinaryAttrs = true

	s.initialize(t)
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
		s.remoteStorage.Close(t)
	})
	s.RunSpanStoreTests(t)
}
