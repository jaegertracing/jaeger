// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestOpenSearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-opensearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Fixtures:     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.OpenSearch(),
		},
	}
	s.e2eInitialize(t, "opensearch")
	s.RunSpanStoreTests(t)
}

func TestOpenSearchStorage_ManualRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	setupManualRolloverIndices(t, "jaeger-mr")
	runRotationSmokeTest(t, "../../config-opensearch-manual-rollover.yaml", "opensearch")
}

func TestOpenSearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	runRotationSmokeTest(t, "../../config-opensearch-auto-rollover.yaml", "opensearch")
}

func TestOpenSearchStorage_DataStream(t *testing.T) {
	t.Skip("data_stream rotation not yet implemented (see RFC 0004 Phase 2)")
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	runRotationSmokeTest(t, "../../config-opensearch-data-stream.yaml", "opensearch")
}
