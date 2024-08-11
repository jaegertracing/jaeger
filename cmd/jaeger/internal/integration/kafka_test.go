// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func modifyConfigEncoding(t *testing.T, configFile string, encoding string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]interface{}
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	exporters := config["exporters"].(map[string]interface{})
	kafka := exporters["kafka"].(map[string]interface{})
	kafka["encoding"] = encoding

	newData, err := yaml.Marshal(config)
	require.NoError(t, err)

	tempFile := filepath.Join(t.TempDir(), "modified_config.yaml")
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err)

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

	// Test OTLP formats
	t.Run("OTLPFormats", func(t *testing.T) {
		collector := &E2EStorageIntegration{
			SkipStorageCleaner:  true,
			ConfigFile:          baseCollectorConfig,
			HealthCheckEndpoint: "http://localhost:8888/metrics",
		}

		// Initialize and start the OTLP collector
		collector.e2eInitialize(t, "kafka")

		ingester := &E2EStorageIntegration{
			ConfigFile: baseIngesterConfig,
			StorageIntegration: integration.StorageIntegration{
				CleanUp:                      purge,
				GetDependenciesReturnsSource: true,
				SkipArchiveTest:              true,
			},
		}

		// Initialize and start the OTLP ingester
		ingester.e2eInitialize(t, "kafka")

		// Run the span store tests for OTLP formats
		ingester.RunSpanStoreTests(t)
	})

	// Test legacy Jaeger formats
	t.Run("LegacyJaegerFormats", func(t *testing.T) {
		legacyCollectorConfig := modifyConfigEncoding(t, baseCollectorConfig, "jaeger_json")
		legacyIngesterConfig := modifyConfigEncoding(t, baseIngesterConfig, "jaeger_json")
		collector := &E2EStorageIntegration{
			SkipStorageCleaner:  true,
			ConfigFile:          legacyCollectorConfig,
			HealthCheckEndpoint: "http://localhost:8888/metrics",
		}

		// Initialize and start the Jaeger collector
		collector.e2eInitialize(t, "kafka")

		ingester := &E2EStorageIntegration{
			ConfigFile: legacyIngesterConfig,
			StorageIntegration: integration.StorageIntegration{
				CleanUp:                      purge,
				GetDependenciesReturnsSource: true,
				SkipArchiveTest:              true,
			},
		}

		// Initialize and start the Jaeger ingester
		ingester.e2eInitialize(t, "kafka")

		// Run the span store tests for legacy Jaeger formats
		ingester.RunSpanStoreTests(t)
	})
}
