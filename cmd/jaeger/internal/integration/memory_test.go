// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestMemoryStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "memory_v2")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
	}
	s.e2eInitialize(t, "memory")
	s.RunAll(t)
}
