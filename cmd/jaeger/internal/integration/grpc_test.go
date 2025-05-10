// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	remoteBackend := &E2EStorageIntegration{
		ConfigFile:      "../../config-remote-storage-backend.yaml",
		HealthCheckPort: 12133,
		MetricsPort:     8887,
		EnvVarOverrides: map[string]string{
			"REMOTE_STORAGE_BACKEND_GRPC_ENDPOINT": "0.0.0.0:4316",
		},
	}
	remoteBackend.e2eInitialize(t, "memory")
	t.Log("Remote backend initialized")

	collector := &E2EStorageIntegration{
		ConfigFile:         "../../config-remote-storage.yaml",
		SkipStorageCleaner: true,
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
		PropagateEnvVars: []string{
			"REMOTE_STORAGE_ENDPOINT",
			"ARCHIVE_REMOTE_STORAGE_ENDPOINT",
		},
	}
	collector.e2eInitialize(t, "grpc")
	t.Log("Collector initialized")

	collector.RunSpanStoreTests(t)
}
