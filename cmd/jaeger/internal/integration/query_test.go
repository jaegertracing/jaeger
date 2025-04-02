// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestJaegerQueryService(t *testing.T) {
	integration.SkipUnlessEnv(t, "query")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-query.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: func(_ *testing.T) {}, // nothing to clean
		},
		HealthCheckPort:    12133, // referencing value in config-query.yaml
		MetricsPort:        8887,
		SkipStorageCleaner: true,
	}
	s.e2eInitialize(t, "memory")
	s.RunTraceReaderSmokeTests(t)
}
