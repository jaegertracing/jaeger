// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestMemoryStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "memory")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config.yaml",
		StorageIntegration: integration.StorageIntegration{
			SkipArchiveTest: true,
			CleanUp:         purge,
		},
	}
	s.e2eInitialize(t, "memory")
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunAll(t)
}
