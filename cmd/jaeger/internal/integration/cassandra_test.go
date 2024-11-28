// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func addCreateSchemaConfig(t *testing.T, configFile string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	var config map[string]any
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	extensionsAny, ok := config["extensions"]
	require.True(t, ok)

	extensions, ok := extensionsAny.(map[string]any)
	require.True(t, ok)

	jaegerStorageAny, ok := extensions["jaeger_storage"]
	require.True(t, ok)

	jaegerStorage, ok := jaegerStorageAny.(map[string]any)
	require.True(t, ok)

	backendsAny, ok := jaegerStorage["backends"]
	require.True(t, ok)

	backends, ok := backendsAny.(map[string]any)
	require.True(t, ok)

	for _, storageName := range []string{"some_storage", "another_storage"} {
		storageAny, ok := backends[storageName]
		require.True(t, ok)

		storage, ok := storageAny.(map[string]any)
		require.True(t, ok)

		cassandraAny, ok := storage["cassandra"]
		require.True(t, ok)

		cassandra, ok := cassandraAny.(map[string]any)
		require.True(t, ok)

		schemaAny, ok := cassandra["schema"]
		require.True(t, ok)

		schema, ok := schemaAny.(map[string]any)
		require.True(t, ok)

		schema["create"] = true
	}

	newData, err := yaml.Marshal(config)
	require.NoError(t, err)
	tempFile := filepath.Join(t.TempDir(), "config-cassandra.yaml")
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err)

	t.Logf("Transformed configuration file %s to %s", configFile, tempFile)
	return tempFile
}

func TestCassandraStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "cassandra")

	configFile := "../../config-cassandra.yaml"

	if os.Getenv("SKIP_APPLY_SCHEMA") == "true" {
		// update config file with create = true, to allow schema creation on fly
		configFile = addCreateSchemaConfig(t, configFile)
	}

	s := &E2EStorageIntegration{
		ConfigFile: configFile,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:                      purge,
			GetDependenciesReturnsSource: true,
			SkipArchiveTest:              true,

			SkipList: integration.CassandraSkippedTests,
		},
	}
	s.e2eInitialize(t, "cassandra")
	s.RunSpanStoreTests(t)
}
