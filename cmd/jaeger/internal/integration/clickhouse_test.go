package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestClickHouseStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "clickhouse")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-clickhouse.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
		},
	}
	s.e2eInitialize(t, "clickhouse")
	s.RunClickHouseTests(t)
}
