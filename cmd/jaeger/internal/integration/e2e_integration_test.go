// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import "testing"

func TestConfigsAreValid(t *testing.T) {
	// Ensure that we can parse the existing configs correctly.
	// This is faster to run than the full integration test.
	validateConfig(t, "../../config-elasticsearch.yaml", "elasticsearch")
	validateConfig(t, "../../config-elasticsearch-manual-rollover.yaml", "elasticsearch")
	validateConfig(t, "../../config-elasticsearch-auto-rollover.yaml", "elasticsearch")
	validateConfig(t, "../../config-opensearch.yaml", "opensearch")
	validateConfig(t, "../../config-opensearch-manual-rollover.yaml", "opensearch")
	validateConfig(t, "../../config-opensearch-auto-rollover.yaml", "opensearch")
	validateConfig(t, "../../config-remote-storage-backend.yaml", "memory")
}

func validateConfig(t *testing.T, configFile string, storage string) {
	createStorageCleanerConfig(t, configFile, storage)
	removeBatchProcessor(t, configFile)
}
