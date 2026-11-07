// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

// clickhouseConfigFile selects the hand-built Distributed schema's config when the e2e harness
// starts a multi-node cluster. The schema is applied before Jaeger starts, so that config disables
// create_schema; the single-node config keeps creating its own tables.
func clickhouseConfigFile() string {
	if os.Getenv("CLICKHOUSE_MULTI_NODE") == "true" {
		return "../../config-clickhouse-multi-node.yaml"
	}
	return "../../config-clickhouse.yaml"
}

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	s := &E2EStorageIntegration{
		ConfigFile:   clickhouseConfigFile(),
		FeatureGates: searchGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ClickHouseE2E(),
		},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunSpanStoreTests(t)
}

func TestClickHouseStorage_BackwardCompatibility(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	runBackwardCompatibilityTests(t, "clickhouse", E2EStorageIntegration{
		ConfigFile: clickhouseConfigFile(),
	}, compatScenario{
		Name: "feature gates disabled on both old writer and new reader",
		Capabilities: capabilities.
			E2EWithoutNativeFilters().
			WithoutPagination().
			WithoutSpanSorting(),
	})
}
