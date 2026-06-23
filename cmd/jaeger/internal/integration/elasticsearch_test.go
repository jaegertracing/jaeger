// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestElasticsearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")

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
	integration.SkipUnlessEnv(t, "elasticsearch")
	setupManualRolloverIndices(t, "jaeger-mr")
	runRotationSmokeTest(t, "../../config-elasticsearch-manual-rollover.yaml", "elasticsearch")
}

func TestElasticsearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	runRotationSmokeTest(t, "../../config-elasticsearch-auto-rollover.yaml", "elasticsearch")
}

// TestElasticsearchStorage_DataStream is a placeholder for the data_stream rotation
// strategy e2e test. Per RFC 0004 Phase 2 (item 14), this test should:
//   - Configure rotation.data_stream for spans (services/dependencies stay periodic)
//   - Verify composable index templates are created (§3.2 of RFC)
//   - Write spans via OTLP, read them back, verify end-to-end
//   - Run against both Elasticsearch and OpenSearch backends
//
// Prerequisites (from RFC 0004 §8, Phase 2):
//   - @timestamp field added to span documents
//   - DataStreamStrategy.CreateTemplates() implemented
//   - DataStreamStrategy.WriteTarget() returns data stream name
//   - DataStreamStrategy.OpType() returns "create"
//   - ISM/ILM policy creation for the data stream lifecycle
//
// No setup helper is needed because data streams auto-create on first write
// once the composable template is in place.
func TestElasticsearchStorage_DataStream(t *testing.T) {
	t.Skip("data_stream rotation not yet implemented (see RFC 0004 Phase 2)")
	integration.SkipUnlessEnv(t, "elasticsearch")
	runRotationSmokeTest(t, "../../config-elasticsearch-data-stream.yaml", "elasticsearch")
}
