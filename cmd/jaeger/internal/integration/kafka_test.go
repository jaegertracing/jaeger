// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

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
	otlpCollectorConfig := "../../collector-with-kafka.yaml"
	otlpIngesterConfig := "../../ingester-remote-storage.yaml"

	// Legacy Jaeger config files
	legacyCollectorConfig := "../../collector-with-jaeger.yaml"
	legacyIngesterConfig := "../../ingester-with-jaeger.yaml"

	// Test OTLP formats
	t.Run("OTLPFormats", func(t *testing.T) {
		collector := &E2EStorageIntegration{
			SkipStorageCleaner:  true,
			ConfigFile:          otlpCollectorConfig,
			HealthCheckEndpoint: "http://localhost:8888/metrics",
		}

		// Initialize and start the OTLP collector
		collector.e2eInitialize(t, "kafka")

		ingester := &E2EStorageIntegration{
			ConfigFile: otlpIngesterConfig,
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
