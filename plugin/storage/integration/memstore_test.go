package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

type MemStorageIntegrationTestSuite struct {
	StorageIntegration
}

func (s *MemStorageIntegrationTestSuite) initialize() error {
	logger, _ := testutils.NewLogger()
	s.logger = logger

	store := memory.NewStore()
	s.SpanReader = store
	s.SpanWriter = store

	// TODO DependencyWriter is not implemented in memory store

	s.Refresh = s.refresh
	s.CleanUp = s.cleanUp
	return nil
}

func (s *MemStorageIntegrationTestSuite) refresh() error {
	return nil
}

func (s *MemStorageIntegrationTestSuite) cleanUp() error {
	return s.initialize()
}

func TestMemoryStorage(t *testing.T) {
	// if os.Getenv("STORAGE") != "memory" {
	// 	t.Skip("Integration test against MemoryStore skipped; set STORAGE env var to 'memory' to run this")
	// }
	s := &MemStorageIntegrationTestSuite{}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}
