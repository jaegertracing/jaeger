package integration

import "testing"

func TestCreateStorageCleanerConfig(t *testing.T) {
	createStorageCleanerConfig(t, "../../config-elasticsearch.yaml", "elasticsearch")
}
