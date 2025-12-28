// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "clickhouse")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-clickhouse.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
			SkipList: []string{
				"Tags_in_one_spot_-_Tags",
				"Tags_in_one_spot_-_Logs",
				"Tags_in_one_spot_-_Logs",
				"Tags_in_different_spots",
			},
		},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunSpanStoreTests(t)
}
