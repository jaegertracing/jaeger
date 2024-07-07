package integration

import "testing"

func TestCreateStorageCleanerConfig(t *testing.T) {
	// Ensure that we can parse the existing configs correctly.
	// This is faster to run than the full integration test.
	createStorageCleanerConfig(t, "../../config-elasticsearch.yaml", "elasticsearch")
	createStorageCleanerConfig(t, "../../config-opensearch.yaml", "opensearch")
}
