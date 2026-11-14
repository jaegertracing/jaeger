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

// runClickHouseStorageSuite runs the span store suite against a Jaeger binary that reads
// configFile. The storage name also names the metrics snapshot of the run.
func runClickHouseStorageSuite(t *testing.T, configFile string, storage string) {
	s := &E2EStorageIntegration{
		ConfigFile:   configFile,
		FeatureGates: searchGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ClickHouseE2E(),
		},
	}
	s.e2eInitialize(t, storage)
	s.RunSpanStoreTests(t)
}

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	runClickHouseStorageSuite(t, "../../config-clickhouse.yaml", "clickhouse")
}

// TestClickHouseStorage_HoistedAttributes runs the same suite against tables with hoisted
// columns, so every search must return what it returns without them. Its storage name keeps
// its metrics snapshot apart from the snapshot of the plain run, which is the one that CI
// compares.
func TestClickHouseStorage_HoistedAttributes(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	integration.RecreateClickHouseDatabase(t, integration.ClickHouseHoistedDatabase)
	runClickHouseStorageSuite(t, writeClickHouseHoistedConfig(t), "clickhouse_hoisted")
}

// writeClickHouseHoistedConfig writes a copy of config-clickhouse.yaml whose backends use the
// database of the hoisted runs and whose trace backend hoists the shared attribute list, and
// returns its path. The copy is derived, not committed, so it cannot drift from the config
// the plain run uses.
func writeClickHouseHoistedConfig(t *testing.T) string {
	data, err := os.ReadFile("../../config-clickhouse.yaml")
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))
	jaegerStorage := config["extensions"].(map[string]any)["jaeger_storage"].(map[string]any)

	hoistedEntries := make([]map[string]string, 0, len(integration.ClickHouseHoistedAttributes))
	for _, attribute := range integration.ClickHouseHoistedAttributes {
		entry := map[string]string{
			"key":    attribute.Key,
			"level":  attribute.Level,
			"type":   attribute.Type,
			"column": attribute.Column,
		}
		if attribute.Codec != "" {
			entry["codec"] = attribute.Codec
		}
		hoistedEntries = append(hoistedEntries, entry)
	}
	traceBackend := jaegerStorage["backends"].(map[string]any)["some-storage"].(map[string]any)["clickhouse"].(map[string]any)
	traceBackend["database"] = integration.ClickHouseHoistedDatabase
	traceBackend["hoisted_attributes"] = hoistedEntries
	metricBackend := jaegerStorage["metric_backends"].(map[string]any)["some-metrics"].(map[string]any)["clickhouse"].(map[string]any)
	metricBackend["database"] = integration.ClickHouseHoistedDatabase

	hoistedYAML, err := yaml.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config-clickhouse-hoisted.yaml")
	require.NoError(t, os.WriteFile(path, hoistedYAML, 0o600))
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
