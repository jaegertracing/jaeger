// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

type GRPCStorageIntegration struct {
	E2EStorageIntegration

	remoteStorage        *integration.RemoteMemoryStorage
	archiveRemoteStorage *integration.RemoteMemoryStorage
}

func (s *GRPCStorageIntegration) initializeRemoteStorages(t *testing.T) {
	s.remoteStorage = integration.StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC)
	s.archiveRemoteStorage = integration.StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC+1)
}

func (s *GRPCStorageIntegration) closeRemoteStorages(t *testing.T) {
	s.remoteStorage.Close(t)
	s.archiveRemoteStorage.Close(t)
}

func (s *GRPCStorageIntegration) cleanUp(t *testing.T) {
	s.closeRemoteStorages(t)
	s.initializeRemoteStorages(t)
}

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	s := &E2EStorageIntegration{
		ConfigFile:         "../../config-remote-storage.yaml",
		SkipStorageCleaner: true,
	}
	s.e2eInitialize(t, "grpc")
	s.RunSpanStoreTests(t)
}
