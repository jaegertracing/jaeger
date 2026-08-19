// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestMemoryStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageMemoryV2)

	s := &E2EStorageIntegration{
		ConfigFile: "../../config.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.NoStructuredFilters(),
		},
	}
	s.e2eInitialize(t, "memory")
	s.RunAll(t)
	// The memory reader evaluates no structured filter, so this is where the query service's
	// rewrite of one into the legacy predicate fields can be checked against a live backend.
	s.RunFilterRewriteTest(t)
}
