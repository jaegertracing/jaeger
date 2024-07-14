// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestKafkaStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "kafka")

	s := &E2EStorageIntegration{
		ConfigFile: "../../collector-with-kafka.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:                      purge,
			GetDependenciesReturnsSource: true,
			SkipArchiveTest:              true,
		},
	}

	s.e2eInitialize(t, "kafka")

	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})

	s.RunSpanStoreTests(t)
}


