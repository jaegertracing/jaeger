// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	t.Setenv("CLICKHOUSE_ATTRIBUTE_METADATA_CACHE_TTL", "5s")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-clickhouse.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.E2EWithoutNativeFilters(),
		},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunSpanStoreTests(t)
}
