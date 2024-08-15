// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func createConfigWithEncoding(t *testing.T, configFile string, targetEncoding string, uniqueTopic string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err, "Failed to read config file: %s", configFile)

	var config map[string]any
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err, "Failed to unmarshal YAML data from config file: %s", configFile)

	// Function to recursively search and replace the encoding
	var replaceEncodingAndTopic func(m map[string]any)
	replaceEncodingAndTopic = func(m map[string]any) {
		for k, v := range m {
			if k == "encoding" {
				oldEncoding := v.(string)
				m[k] = targetEncoding
				t.Logf("Replaced encoding '%s' with '%s' in key: %s", oldEncoding, targetEncoding, k)
			} else if k == "topic" {
				oldTopic := v.(string)
				m[k] = uniqueTopic
				t.Logf("Replaced topic '%s' with '%s' in key: %s", oldTopic, uniqueTopic, k)
			} else if subMap, ok := v.(map[string]any); ok {
				replaceEncodingAndTopic(subMap)
			}
		}
	}

	replaceEncodingAndTopic(config)

	newData, err := yaml.Marshal(config)
	require.NoError(t, err, "Failed to marshal YAML data after encoding replacement")

	tempFile := filepath.Join(t.TempDir(), fmt.Sprintf("config_%s.yaml", targetEncoding))
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err, "Failed to write updated config file to: %s", tempFile)

	t.Logf("Configuration file with encoding '%s' and topic '%s' written to: %s", targetEncoding, uniqueTopic, tempFile)

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

	formats := []struct {
		name     string
		encoding string
	}{
		{"OTLP Proto", "otlp_proto"},
		{"OTLP JSON", "otlp_json"},
		{"Jaeger Proto", "jaeger_proto"},
		{"Jaeger JSON", "jaeger_json"},
		{"Jaeger Thrift", "jaeger_thrift"},
	}

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			t.Logf("Starting %s test", format.name)

			// Generate a unique topic name for test runs
			uniqueTopic := fmt.Sprintf("jaeger-spans-%d", time.Now().UnixNano())
			t.Logf("Using unique Kafka topic: %s", uniqueTopic)

			collectorConfig := createConfigWithEncoding(t, baseCollectorConfig, format.encoding, uniqueTopic)
			ingesterConfig := createConfigWithEncoding(t, baseIngesterConfig, format.encoding, uniqueTopic)

			collector := &E2EStorageIntegration{
				SkipStorageCleaner:  true,
				ConfigFile:          collectorConfig,
				HealthCheckEndpoint: "http://localhost:8888/metrics",
			}
			collector.e2eInitialize(t, "kafka")
			t.Logf("Collector initialized with %s format", format.name)

			ingester := &E2EStorageIntegration{
				ConfigFile: ingesterConfig,
				StorageIntegration: integration.StorageIntegration{
					CleanUp:                      purge,
					GetDependenciesReturnsSource: true,
					SkipArchiveTest:              true,
				},
			}
			ingester.e2eInitialize(t, "kafka")
			t.Logf("Ingester initialized with %s format", format.name)
			ingester.RunSpanStoreTests(t)
			t.Logf("%s test completed successfully", format.name)
		})
	}
}
