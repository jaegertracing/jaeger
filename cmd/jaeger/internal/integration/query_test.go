// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestJaegerQueryService(t *testing.T) {
	integration.SkipUnlessEnv(t, "query")

	// Start instance of Jaeger with jaeger_query reading from Remote Storage, which
	// will be started in GRPCStorageIntegration below
	query := &E2EStorageIntegration{
		ConfigFile:         "../../config-query.yaml",
		SkipStorageCleaner: true,
	}
	query.e2eInitialize(t, "grpc")
	t.Log("Query initialized")

	// Start another instance of Jaeger receiving traces and write traces to Remote Storage
	collector := &E2EStorageIntegration{
		ConfigFile:      "../../config-remote-storage-backend.yaml",
		HealthCheckPort: 12133,
		MetricsPort:     8887,
		EnvVarOverrides: map[string]string{
			"JAEGER_QUERY_GRPC_ENDPOINT": "localhost:0",
			"JAEGER_QUERY_HTTP_ENDPOINT": "localhost:0",
		},
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
	}
	collector.e2eInitialize(t, "memory")
	t.Log("Collector initialized")

	collector.RunSpanStoreTests(t)
}
