// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

// clickhouseConfigFile returns ../../config-clickhouse.yaml in single-node mode, or a derived
// copy configured for the Distributed layout (replicated + table_engine.cluster) in multi-node mode.
func clickhouseConfigFile(t *testing.T) string {
	if os.Getenv("CLICKHOUSE_MULTI_NODE") == "true" {
		return writeClickHouseMultiNodeConfig(t)
	}
	return "../../config-clickhouse.yaml"
}

func writeClickHouseMultiNodeConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../config-clickhouse.yaml")
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))
	jaegerStorage := config["extensions"].(map[string]any)["jaeger_storage"].(map[string]any)
	traceBackend := jaegerStorage["backends"].(map[string]any)["some-storage"].(map[string]any)["clickhouse"].(map[string]any)
	traceBackend["table_engine"] = map[string]any{
		"replicated": map[string]any{},
		"cluster": map[string]any{
			"name": "jaeger_cluster",
		},
	}
	multiNodeYAML, err := yaml.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config-clickhouse-multi-node.yaml")
	require.NoError(t, os.WriteFile(path, multiNodeYAML, 0o600))
	return path
}

// runClickHouseStorageSuite runs the span store suite against a Jaeger binary that reads
// configFile. The storage name also names the metrics snapshot of the run.
func runClickHouseStorageSuite(t *testing.T, configFile string, storage string) {
	cleanUp := purge
	if os.Getenv("CLICKHOUSE_MULTI_NODE") == "true" {
		opts := clickhouseConnOptions(t, configFile)
		cleanUp = func(t *testing.T) {
			purge(t)
			// Factory.Purge issues TRUNCATE on public Distributed tables, which does not clear rows
			// stored in the backing *_local tables; truncate them explicitly so tests stay isolated.
			integration.TruncateClickHouseLocalTables(t, opts.Addr, opts.Auth.Database, opts.Auth.Username, opts.Auth.Password)
		}
	}
	s := &E2EStorageIntegration{
		ConfigFile:   configFile,
		FeatureGates: paginationGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      cleanUp,
			Capabilities: capabilities.ClickHouseE2E(),
		},
	}
	s.e2eInitialize(t, storage)
	s.RunSpanStoreTests(t)
}

func clickhouseConnOptions(t *testing.T, configFile string) *clickhouse.Options {
	t.Helper()
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))
	jaegerStorage := config["extensions"].(map[string]any)["jaeger_storage"].(map[string]any)
	traceBackend := jaegerStorage["backends"].(map[string]any)["some-storage"].(map[string]any)["clickhouse"].(map[string]any)
	rawAddrs := traceBackend["addresses"].([]any)
	addresses := make([]string, 0, len(rawAddrs))
	for _, a := range rawAddrs {
		addresses = append(addresses, a.(string))
	}
	database := traceBackend["database"].(string)
	basic := traceBackend["auth"].(map[string]any)["basic"].(map[string]any)
	return &clickhouse.Options{
		Addr: addresses,
		Auth: clickhouse.Auth{
			Database: database,
			Username: basic["username"].(string),
			Password: basic["password"].(string),
		},
	}
}

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	runClickHouseStorageSuite(t, clickhouseConfigFile(t), "clickhouse")
}

// runClickHouseStorageSuiteWithLayout runs the span store suite against a database that it
// creates anew, whose tables are in attributeLayout and have the hoisted columns. Each such
// run needs a database of its own, because the backend neither adds a column to an existing
// table nor converts its layout. The storage
// name keeps the metrics snapshot of the run apart from the snapshot of the plain run, which
// is the one that CI compares.
func runClickHouseStorageSuiteWithLayout(t *testing.T, database string, attributeLayout string, storage string) {
	integration.SkipUnlessEnv(t, integration.StorageClickHouse)
	if os.Getenv("CLICKHOUSE_MULTI_NODE") == "true" {
		t.Skip("The multi-node suite owns the Distributed schema in jaeger")
	}
	integration.RecreateClickHouseDatabase(t, database)
	config := writeClickHouseHoistedConfig(t, database, attributeLayout)
	runClickHouseStorageSuite(t, config, storage)
}

// TestClickHouseStorage_HoistedAttributes runs the same suite against tables with hoisted
// columns, so every search must return what it returns without them.
func TestClickHouseStorage_HoistedAttributes(t *testing.T) {
	runClickHouseStorageSuiteWithLayout(t, "jaeger_hoisted", "nested", "clickhouse_hoisted")
}

// TestClickHouseStorage_MapLayout runs the same suite against tables in the map attribute
// layout, which also have the hoisted columns.
func TestClickHouseStorage_MapLayout(t *testing.T) {
	runClickHouseStorageSuiteWithLayout(t, "jaeger_map", "map", "clickhouse_map")
}

// writeClickHouseHoistedConfig writes a copy of config-clickhouse.yaml whose backends use
// database and whose trace backend stores attributes in attributeLayout and hoists the shared
// attribute list, and returns its path. The copy is derived, not committed, so it cannot
// drift from the config the plain run uses.
func writeClickHouseHoistedConfig(t *testing.T, database string, attributeLayout string) string {
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
	traceBackend["database"] = database
	traceBackend["attribute_layout"] = attributeLayout
	traceBackend["hoisted_attributes"] = hoistedEntries
	metricBackend := jaegerStorage["metric_backends"].(map[string]any)["some-metrics"].(map[string]any)["clickhouse"].(map[string]any)
	metricBackend["database"] = database
	// The metric backend opens the same tables, so it has to name their layout as well.
	metricBackend["attribute_layout"] = attributeLayout

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
