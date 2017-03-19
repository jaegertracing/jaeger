package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
)

// InitMockCollector initializes a MockTCollector fixture
func InitMockCollector(t *testing.T) (*metrics.LocalFactory, *MockTCollector) {
	factory := metrics.NewLocalFactory(0)
	collector, err := StartMockTCollector()
	require.NoError(t, err)

	return factory, collector
}
