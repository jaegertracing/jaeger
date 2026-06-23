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

// TestElasticsearchStorage_ManualRollover validates the full config → storage → query
// pipeline using the manual_rollover rotation strategy. It creates the initial
// indices and aliases that Jaeger expects, then writes and reads traces.
func TestElasticsearchStorage_ManualRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")
	setupManualRolloverIndices(t, "jaeger-mr")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch-manual-rollover.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ElasticsearchSmokeTest(),
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	s.RunSpanStoreTests(t)
}

// TestElasticsearchStorage_AutoRollover validates the full config → storage → query
// pipeline using the auto_rollover rotation strategy with an ILM/ISM policy.
func TestElasticsearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch-auto-rollover.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ElasticsearchSmokeTest(),
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	s.RunSpanStoreTests(t)
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
// The test config would use:
//
//	indices:
//	  index_prefix: "jaeger-ds"
//	  spans:
//	    rotation:
//	      data_stream:
//	        policy_name: "jaeger-spans-lifecycle"
//	  services:
//	    rotation:
//	      periodic: { date_layout: "2006-01-02" }
//
// No setup helper is needed because data streams auto-create on first write
// once the composable template is in place.
func TestElasticsearchStorage_DataStream(t *testing.T) {
	t.Skip("data_stream rotation not yet implemented (see RFC 0004 Phase 2)")
}
