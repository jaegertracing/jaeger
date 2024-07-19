// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestKafkaStorage(t *testing.T) {
    integration.SkipUnlessEnv(t, "kafka")

    collectorConfig := "../../collector-with-kafka.yaml"
    ingesterConfig := "../../ingester-remote-storage.yaml"

    collector := &E2EStorageIntegration{
        ConfigFile: collectorConfig,
        StorageIntegration: integration.StorageIntegration{
            GetDependenciesReturnsSource: true,
            SkipArchiveTest: true,
        },
    }

    // Initialize and start the collector
    collector.e2eInitialize(t, "kafka-collector", false)


    ingester := &E2EStorageIntegration{
        ConfigFile: ingesterConfig,
        StorageIntegration: integration.StorageIntegration{
            CleanUp: purge,
            GetDependenciesReturnsSource: true,
            SkipArchiveTest: true,
        },
    }

	// Initialize and start the ingester
    ingester.e2eInitialize(t, "kafka-ingester", true)

    // Set up cleanup for both collector and ingester
    t.Cleanup(func() {
        collector.e2eCleanUp(t)
        ingester.e2eCleanUp(t)
    })

    // Run the span store tests
    ingester.RunSpanStoreTests(t)
}