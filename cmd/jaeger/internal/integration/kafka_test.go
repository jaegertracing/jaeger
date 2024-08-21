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

// createConfigWithEncoding rewrites the base configuration files to use the given encoding
// and Kafka topic which are varied between the test runs.
//
// Once OTEL Collector supports default values for env vars
// (https://github.com/open-telemetry/opentelemetry-collector/issues/5228)
// we can change the config to use topic: "${KAFKA_TOPIC:-jaeger-spans}"
// and export a KAFKA_TOPIC var with random topic name in the tests.
func createConfigWithEncoding(t *testing.T, configFile string, targetEncoding string, uniqueTopic string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err, "Failed to read config file: %s", configFile)

	var config map[string]any
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err, "Failed to unmarshal YAML data from config file: %s", configFile)

	// Function to recursively search and replace the encoding and topic
	var replaceEncodingAndTopic func(m map[string]any) int
	replaceEncodingAndTopic = func(m map[string]any) int {
		replacements := 0
		for k, v := range m {
			if k == "encoding" {
				oldEncoding := v.(string)
				m[k] = targetEncoding
				t.Logf("Replaced encoding '%s' with '%s' in key: %s", oldEncoding, targetEncoding, k)
				replacements++
			} else if k == "topic" {
				oldTopic := v.(string)
				m[k] = uniqueTopic
				t.Logf("Replaced topic '%s' with '%s' in key: %s", oldTopic, uniqueTopic, k)
				replacements++
			} else if subMap, ok := v.(map[string]any); ok {
				replacements += replaceEncodingAndTopic(subMap)
			}
		}
		return replacements
	}

	totalReplacements := replaceEncodingAndTopic(config)
	require.Equal(t, 2, totalReplacements, "Expected exactly 2 replacements (encoding and topic), but got %d", totalReplacements)

	newData, err := yaml.Marshal(config)
	require.NoError(t, err, "Failed to marshal YAML data after encoding replacement")

	tempFile := filepath.Join(t.TempDir(), fmt.Sprintf("config_%s.yaml", targetEncoding))
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err, "Failed to write updated config file to: %s", tempFile)

	t.Logf("Transformed configuration file %s to %s", configFile, tempFile)
	return tempFile
}

func TestKafkaStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "kafka")

	tests := []struct {
		encoding string
		skip     string
	}{
		{encoding: "otlp_proto"},
		{encoding: "otlp_json", skip: "not supported: https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/33627"},
		{encoding: "jaeger_proto"},
		{encoding: "jaeger_json"},
	}

	for _, test := range tests {
		t.Run(test.encoding, func(t *testing.T) {
			if test.skip != "" {
				t.Skip(test.skip)
			}
			uniqueTopic := fmt.Sprintf("jaeger-spans-%d", time.Now().UnixNano())
			t.Logf("Using unique Kafka topic: %s", uniqueTopic)

			// Unlike the other storage tests where "collector" has access to the storage,
			// here we have two distinct binaries, collector and ingester, and only the ingester
			// has access to the storage and allows the test to query it.
			// We reuse E2EStorageIntegration struct to manage lifecycle of the collector,
			// but the tests are run against the ingester.

			collector := &E2EStorageIntegration{
				BinaryName:         "jaeger-v2-collector",
				ConfigFile:         createConfigWithEncoding(t, "../../config-kafka-collector.yaml", test.encoding, uniqueTopic),
				SkipStorageCleaner: true,
			}
			collector.e2eInitialize(t, "kafka")
			t.Log("Collector initialized")

			ingester := &E2EStorageIntegration{
				BinaryName:          "jaeger-v2-ingester",
				ConfigFile:          createConfigWithEncoding(t, "../../config-kafka-ingester.yaml", test.encoding, uniqueTopic),
				HealthCheckEndpoint: "http://localhost:14133/status",
				StorageIntegration: integration.StorageIntegration{
					CleanUp:                      purge,
					GetDependenciesReturnsSource: true,
					SkipArchiveTest:              true,
				},
			}
			ingester.e2eInitialize(t, "kafka")
			t.Log("Ingester initialized")

			ingester.RunSpanStoreTests(t)
		})
	}
}
