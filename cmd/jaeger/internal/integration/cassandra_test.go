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
			CleanUp:                      cleanUp,
			GetDependenciesReturnsSource: true,
			SkipArchiveTest:              true,

			SkipList: []string{
				"Tags_+_Operation_name_+_Duration_range",
				"Tags_+_Duration_range",
				"Tags_+_Operation_name_+_max_Duration",
				"Tags_+_max_Duration",
				"Operation_name_+_Duration_range",
				"Duration_range",
				"max_Duration",
				"Multiple_Traces",
			},
		},
	}
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}
