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
	integration.SkipTestIfNeeded(t, false)
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
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
	integration.SkipTestIfNeeded(t, false)
	setupManualRolloverIndices(t, "jaeger-mr")
	runRotationSmokeTest(t, "../../config-elasticsearch-manual-rollover.yaml", "elasticsearch", func(t *testing.T) {
		initManualRolloverIndices(t, "jaeger-mr")
	})
}

func TestElasticsearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	integration.SkipTestIfNeeded(t, false)
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	runRotationSmokeTest(t, "../../config-elasticsearch-auto-rollover.yaml", "elasticsearch", func(t *testing.T) {
		initAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	})
}

func TestElasticsearchStorage_DataStream(t *testing.T) {
	t.Skip("data_stream rotation not yet implemented (see RFC 0004 Phase 2)")

	// No setup helper is needed because data streams auto-create on first write
	// once the composable template is in place.
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	integration.SkipTestIfNeeded(t, false)
	runRotationSmokeTest(t, "../../config-elasticsearch-data-stream.yaml", "elasticsearch", func(*testing.T) {})
}

func TestElasticSearch_BackwardsCompatibility(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	integration.SkipTestIfNeeded(t, true)
	s := E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
		},
	}
	runBackwardCompatibilityTests(t, s, capabilities.Elasticsearch(), capabilities.Elasticsearch())
}
