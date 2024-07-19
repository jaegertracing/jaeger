// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestCassandraStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "cassandra")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-cassandra.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:                      purge,
			GetDependenciesReturnsSource: true,
			SkipArchiveTest:              true,

			SkipList: integration.CassandraSkippedTests,
		},
	}
	s.e2eInitialize(t, "cassandra", true)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}
