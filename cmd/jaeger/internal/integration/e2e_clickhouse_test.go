// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	s := &E2EStorageIntegration{
		ConfigFile:   "../../config-clickhouse.yaml",
		FeatureGates: searchGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ClickHouseE2E(),
		},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunSpanStoreTests(t)
}

// TestClickHouseStorage_HoistedAttributes runs the same suite against tables that copy the
// searched attributes into dedicated columns, so every search must return what it returns
// without them.
func TestClickHouseStorage_HoistedAttributes(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	integration.RecreateClickHouseDatabase(t, integration.ClickHouseHoistedDatabase)
	s := &E2EStorageIntegration{
		ConfigFile:   clickHouseHoistedConfig(t),
		FeatureGates: searchGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ClickHouseE2E(),
		},
	}
	// The name keeps the metrics snapshot of this run apart from the snapshot of the plain run,
	// which is the one that CI compares.
	s.e2eInitialize(t, "clickhouse_hoisted")
	s.RunSpanStoreTests(t)
}

// clickHouseHoistedConfig writes a copy of config-clickhouse.yaml whose backends use the
// database of the hoisted runs and whose trace backend hoists the shared attribute list. The
// copy is derived, not committed, so it cannot drift from the config the plain run uses.
func clickHouseHoistedConfig(t *testing.T) string {
	data, err := os.ReadFile("../../config-clickhouse.yaml")
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))
	jaegerStorage := config["extensions"].(map[string]any)["jaeger_storage"].(map[string]any)

	hoisted := make([]map[string]string, 0, len(integration.ClickHouseHoistedAttributes))
	for _, h := range integration.ClickHouseHoistedAttributes {
		entry := map[string]string{
			"key":    h.Key,
			"level":  h.Level,
			"type":   h.Type,
			"column": h.Column,
		}
		if h.Codec != "" {
			entry["codec"] = h.Codec
		}
		hoisted = append(hoisted, entry)
	}
	traces := jaegerStorage["backends"].(map[string]any)["some-storage"].(map[string]any)["clickhouse"].(map[string]any)
	traces["database"] = integration.ClickHouseHoistedDatabase
	traces["hoisted_attributes"] = hoisted
	metrics := jaegerStorage["metric_backends"].(map[string]any)["some-metrics"].(map[string]any)["clickhouse"].(map[string]any)
	metrics["database"] = integration.ClickHouseHoistedDatabase

	out, err := yaml.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config-clickhouse-hoisted.yaml")
	require.NoError(t, os.WriteFile(path, out, 0o600))
	return path
}

func TestClickHouseStorage_BackwardCompatibility(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	runBackwardCompatibilityTests(t, "clickhouse", E2EStorageIntegration{
		ConfigFile: "../../config-clickhouse.yaml",
	}, compatScenario{
		Name: "feature gates disabled on both old writer and new reader",
		Capabilities: capabilities.
			E2EWithoutNativeFilters().
			WithoutPagination().
			WithoutSpanSorting(),
	})
}
