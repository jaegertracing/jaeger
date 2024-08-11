// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func modifyConfigEncoding(t *testing.T, configFile string, encoding string) string {
	// Read the base configuration file
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	// Unmarshal the YAML into a generic map
	var config map[string]any
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	// Navigate to the Kafka exporter section and modify the encoding
	exportersAny, ok := config["exporters"]
	require.True(t, ok)
	exporters := exportersAny.(map[string]any)

	kafkaExporterAny, ok := exporters["kafka"]
	require.True(t, ok)
	kafkaExporter := kafkaExporterAny.(map[string]any)

	kafkaExporter["encoding"] = encoding

	// Marshal the modified configuration back to YAML
	newData, err := yaml.Marshal(config)
	require.NoError(t, err)

	// Write the modified configuration to a temporary file
	tempFile := filepath.Join(t.TempDir(), fmt.Sprintf("config_with_%s_encoding.yaml", encoding))
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

	baseCollectorConfig := "../../collector-with-kafka.yaml"
	baseIngesterConfig := "../../ingester-remote-storage.yaml"

	runTest := func(t *testing.T, encoding string) {
		collectorConfig := modifyConfigEncoding(t, baseCollectorConfig, encoding)
		ingesterConfig := modifyConfigEncoding(t, baseIngesterConfig, encoding)

		collector := &E2EStorageIntegration{
			SkipStorageCleaner:  true,
			ConfigFile:          collectorConfig,
			HealthCheckEndpoint: "http://localhost:8888/metrics",
		}
		collector.e2eInitialize(t, "kafka")

		ingester := &E2EStorageIntegration{
			ConfigFile: ingesterConfig,
			StorageIntegration: integration.StorageIntegration{
				CleanUp:                      purge,
				GetDependenciesReturnsSource: true,
				SkipArchiveTest:              true,
			},
		}
		ingester.e2eInitialize(t, "kafka")

		ingester.RunSpanStoreTests(t)
	}

	t.Run("OTLPFormats", func(t *testing.T) {
		runTest(t, "otlp_proto")
	})

	t.Run("LegacyJaegerFormats", func(t *testing.T) {
		runTest(t, "jaeger_json")
	})
}
