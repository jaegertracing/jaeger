// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestOpenSearchStorageWithCompression(t *testing.T) {
	integration.SkipUnlessEnv(t, "opensearch")

	// This test specifically verifies that OpenSearch works with compression enabled
	// It addresses issue #7200 where compression broke template creation
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-opensearch-compression.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:                      purge,
			Fixtures:                     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			GetOperationsMissingSpanKind: true,
		},
	}
	s.e2eInitialize(t, "opensearch")

	// The test will fail during initialization if the compression bug exists
	// because template creation will fail with "Compressor detection" error
	s.RunSpanStoreTests(t)
}
