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

func createConfigWithEncoding(t *testing.T, configFile string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err, "Failed to read config file: %s", configFile)

	var config map[string]any
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err, "Failed to unmarshal YAML data from config file: %s", configFile)

	// Function to recursively search and replace the encoding
	var replaceEncoding func(m map[string]any)
	replaceEncoding = func(m map[string]any) {
		for k, v := range m {
			if k == "encoding" && v == "otlp_proto" {
				t.Logf("Replacing encoding 'otlp_proto' with 'jaeger_json' in key: %s", k)
				m[k] = "jaeger_json"
			} else if subMap, ok := v.(map[string]any); ok {
				replaceEncoding(subMap)
			}
		}
	}

	replaceEncoding(config)

	newData, err := yaml.Marshal(config)
	require.NoError(t, err, "Failed to marshal YAML data after encoding replacement")

	tempFile := filepath.Join(t.TempDir(), "jaeger_encoding_config.yaml")
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err, "Failed to write updated config file to: %s", tempFile)

	t.Logf("Configuration file with updated encoding written to: %s", tempFile)

	return tempFile
}

func TestKafkaStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "kafka")

	// TODO these config files use topic: "jaeger-spans",
	// but for integration tests we want to use random topic in each run.
	// https://github.com/jaegertracing/jaeger/blob/ed5cc2981c34158d0650cb96cb2fafcb753bea70/plugin/storage/integration/kafka_test.go#L50-L51
	// Once OTEL Collector supports default values for env vars
	// (https://github.com/open-telemetry/opentelemetry-collector/issues/5228)
	// we can change the config to use topic: "${KAFKA_TOPIC:-jaeger-spans}"
	// and export a KAFKA_TOPIC var with random topic name in the tests.

	// OTLP config files
	baseCollectorConfig := "../../collector-with-kafka.yaml"
	baseIngesterConfig := "../../ingester-remote-storage.yaml"

	// Test OTLP formats (no need to modify as default is otlp_proto)
	t.Run("OTLPFormats", func(t *testing.T) {
		t.Log("Starting OTLPFormats test")
		collector := &E2EStorageIntegration{
			SkipStorageCleaner:  true,
			ConfigFile:          baseCollectorConfig,
			HealthCheckEndpoint: "http://localhost:8888/metrics",
		}
		collector.e2eInitialize(t, "kafka")
		t.Log("Collector initialized")

		ingester := &E2EStorageIntegration{
			ConfigFile: baseIngesterConfig,
			StorageIntegration: integration.StorageIntegration{
				CleanUp:                      purge,
				GetDependenciesReturnsSource: true,
				SkipArchiveTest:              true,
			},
		}
		ingester.e2eInitialize(t, "kafka")
		t.Log("Ingester initialized")
		ingester.RunSpanStoreTests(t)
		t.Log("OTLPFormats test completed successfully")
	})

	// Test legacy Jaeger formats
	t.Run("LegacyJaegerFormats", func(t *testing.T) {
		t.Log("Starting LegacyJaegerFormats test")

		collectorConfig := createConfigWithEncoding(t, baseCollectorConfig)
		ingesterConfig := createConfigWithEncoding(t, baseIngesterConfig)

		collector := &E2EStorageIntegration{
			SkipStorageCleaner:  true,
			ConfigFile:          collectorConfig,
			HealthCheckEndpoint: "http://localhost:8888/metrics",
		}
		collector.e2eInitialize(t, "kafka")
		t.Log("Collector initialized with legacy Jaeger formats")

		ingester := &E2EStorageIntegration{
			ConfigFile: ingesterConfig,
			StorageIntegration: integration.StorageIntegration{
				CleanUp:                      purge,
				GetDependenciesReturnsSource: true,
				SkipArchiveTest:              true,
			},
		}
		ingester.e2eInitialize(t, "kafka")
		t.Log("Ingester initialized with legacy Jaeger formats")
		ingester.RunSpanStoreTests(t)
		t.Log("LegacyJaegerFormats test completed successfully")
	})
}
