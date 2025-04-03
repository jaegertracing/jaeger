// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestJaegerQueryService(t *testing.T) {
	integration.SkipUnlessEnv(t, "query")

	// Start instance of Jaeger with jaeger_query reading from remote storage
	query := &E2EStorageIntegration{
		ConfigFile:         "../../config-query.yaml",
		SkipStorageCleaner: true,
		// referencing values in config-query.yaml
		HealthCheckPort: 12133,
		MetricsPort:     8887,
	}
	query.e2eInitialize(t, "grpc")
	t.Log("Query initialized")

	// Start another instance of Jaeger receiving traces from OTLP and write traces to remote storage
	collector := &GRPCStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile:         "../../config-remote-storage-without-query.yaml",
			SkipStorageCleaner: true,
		},
	}
	collector.CleanUp = collector.cleanUp
	collector.initializeRemoteStorages(t)
	collector.e2eInitialize(t, "grpc")
	t.Cleanup(func() {
		collector.remoteStorage.Close(t)
		collector.archiveRemoteStorage.Close(t)
	})
	t.Log("Collector initialized")

	collector.RunSpanStoreTests(t)
}
