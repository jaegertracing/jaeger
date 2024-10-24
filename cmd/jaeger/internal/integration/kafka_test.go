// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestKafkaStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "kafka")

	tests := []struct {
		encoding string
		skip     string
	}{
		{encoding: "otlp_proto"},
		{encoding: "otlp_json"},
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
			envVarOverrides := map[string]string{
				"KAFKA_TOPIC":    uniqueTopic,
				"KAFKA_ENCODING": test.encoding,
			}

			collector := &E2EStorageIntegration{
				BinaryName:         "jaeger-v2-collector",
				ConfigFile:         "../../config-kafka-collector.yaml",
				SkipStorageCleaner: true,
				EnvVarOverrides:    envVarOverrides,
			}
			collector.e2eInitialize(t, "kafka")
			t.Log("Collector initialized")

			ingester := &E2EStorageIntegration{
				BinaryName:          "jaeger-v2-ingester",
				ConfigFile:          "../../config-kafka-ingester.yaml",
				HealthCheckEndpoint: "http://localhost:14133/status",
				StorageIntegration: integration.StorageIntegration{
					CleanUp:                      purge,
					GetDependenciesReturnsSource: true,
					SkipArchiveTest:              true,
				},
				EnvVarOverrides: envVarOverrides,
			}
			ingester.e2eInitialize(t, "kafka")
			t.Log("Ingester initialized")

			ingester.RunSpanStoreTests(t)
		})
	}
}
