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
				"Tags_in_one_spot_-_Process",
				"Tags_in_different_spots",
				"Tags_+_Operation_name",
				"Tags_+_Operation_name_+_max_Duration",
				"Tags_+_Operation_name_+_Duration_range",
				"Tags_+_Duration_range",
				"Tags_+_max_Duration",
			},
		},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunSpanStoreTests(t)
}
