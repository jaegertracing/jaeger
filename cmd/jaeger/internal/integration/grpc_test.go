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
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
	}
	remoteBackend.e2eInitialize(t, "memory")
	t.Log("Remote backend initialized")

	collector := &E2EStorageIntegration{
		ConfigFile:         "../../config-remote-storage.yaml",
		SkipStorageCleaner: true,
		EnvVarOverrides: map[string]string{
			"JAEGER_QUERY_GRPC_ENDPOINT": "localhost:0",
			"JAEGER_QUERY_HTTP_ENDPOINT": "localhost:0",
		},
	}
	collector.e2eInitialize(t, "grpc")
	t.Log("Collector initialized")

	collector.RunSpanStoreTests(t)
}
