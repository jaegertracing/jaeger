// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-clickhouse.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
		FeatureGates: []string{"jaeger.clickhouse"},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunSpanStoreTests(t)
}
