// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

func TestJaegerQueryService(t *testing.T) {
	integration.SkipUnlessEnv(t, "query")

	// Start instance of Jaeger with jaeger_query reading from Remote Storage, which
	// will be started in GRPCStorageIntegration below
	query := &E2EStorageIntegration{
		ConfigFile:         "../../config-query.yaml",
		SkipStorageCleaner: true,
		// referencing values in config-query.yaml
		HealthCheckPort: 12133,
		MetricsPort:     8887,
	}
	query.e2eInitialize(t, "grpc")
	t.Log("Query initialized")

	// Start another instance of Jaeger receiving traces and write traces to Remote Storage
	collector := &GRPCStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile:         "../../config-remote-storage.yaml",
			SkipStorageCleaner: true,
			EnvVarOverrides: map[string]string{
				// Run jaeger_query on different ports here to avoid conflict
				// with jaeger_query instance of Jaeger above
				"JAEGER_QUERY_GRPC_PORT": fmt.Sprintf("%d", ports.QueryGRPC+1000),
				"JAEGER_QUERY_HTTP_PORT": fmt.Sprintf("%d", ports.QueryHTTP+1000),
			},
		},
	}
	collector.CleanUp = collector.cleanUp
	collector.initializeRemoteStorages(t)
	collector.e2eInitialize(t, "grpc")
	t.Cleanup(func() { collector.closeRemoteStorages(t) })
	t.Log("Collector initialized")

	collector.RunSpanStoreTests(t)
}
