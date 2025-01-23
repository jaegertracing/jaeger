// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

type GRPCStorageIntegration struct {
	E2EStorageIntegration

	remoteStorage        *integration.RemoteMemoryStorage
	archiveRemoteStorage *integration.RemoteMemoryStorage
}

func (s *GRPCStorageIntegration) initialize(t *testing.T) {
	s.remoteStorage = integration.StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC)
	s.archiveRemoteStorage = integration.StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC+1)
}

func (s *GRPCStorageIntegration) cleanUp(t *testing.T) {
	s.remoteStorage.Close(t)
	s.archiveRemoteStorage.Close(t)
	s.initialize(t)
}

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	s := &GRPCStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "../../config-remote-storage.yaml",
		},
	}
	s.CleanUp = s.cleanUp
	s.initialize(t)
	s.e2eInitialize(t, "grpc")
	t.Cleanup(func() {
		s.remoteStorage.Close(t)
	})
	s.RunSpanStoreTests(t)
}
