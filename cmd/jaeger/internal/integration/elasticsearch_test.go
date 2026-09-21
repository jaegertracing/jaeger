// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestElasticsearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)

	s := &E2EStorageIntegration{
		ConfigFile:   "../../config-elasticsearch.yaml",
		FeatureGates: elasticsearchFilterGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Fixtures:     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.Elasticsearch(),
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	s.RunSpanStoreTests(t)
}

func TestElasticsearchStorage_ManualRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	setupManualRolloverIndices(t, "jaeger-mr")
	runRotationSmokeTest(t, "../../config-elasticsearch-manual-rollover.yaml", "elasticsearch", func(t *testing.T) {
		initManualRolloverIndices(t, "jaeger-mr")
	})
}

func TestElasticsearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	runRotationSmokeTest(t, "../../config-elasticsearch-auto-rollover.yaml", "elasticsearch", func(t *testing.T) {
		initAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	})
}

func TestElasticsearchStorage_DataStream(t *testing.T) {
	// No setup helper is needed because data streams auto-create on first write
	// once the composable template is in place.
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	runRotationSmokeTest(t, "../../config-elasticsearch-data-stream.yaml", "elasticsearch", func(*testing.T) {})
}

func TestElasticsearchStorage_BackwardCompatibility(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	runBackwardCompatibilityTests(t, "elasticsearch", E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
		},
	},
		compatScenario{
			Name:         "feature gates disabled on both old writer and new reader",
			OldGates:     nil,
			NewGates:     structuredFilterGates,
			Capabilities: capabilities.Elasticsearch().WithoutTypedAttributeIndexing(),
		},
		compatScenario{
			Name:         "feature gates enabled on new reader only (enable-on-upgrade)",
			OldGates:     nil,
			NewGates:     elasticsearchFilterGates,
			Capabilities: capabilities.Elasticsearch().WithoutTypedAttributeIndexing(),
		},
		compatScenario{
			Name:         "feature gates enabled on both old writer and new reader (already enabled)",
			OldGates:     elasticsearchFilterGates,
			NewGates:     elasticsearchFilterGates,
			Capabilities: capabilities.Elasticsearch(),
		},
	)
}
