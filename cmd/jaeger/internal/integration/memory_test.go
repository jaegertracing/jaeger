// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestMemoryStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageMemoryV2)

	s := &E2EStorageIntegration{
		ConfigFile:         "../../config.yaml",
		RequireCoreMetrics: true, // golden-list BC check; see metrics_compat.go / #6278
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
	}
	s.e2eInitialize(t, "memory")
	s.RunAll(t)
}
