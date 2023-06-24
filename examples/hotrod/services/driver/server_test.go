package driver

import (
	"context"
	"testing"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestFindNearest(t *testing.T) {
	logger := zap.NewNop()
	zapLogger := logger.With(zap.String("service", "test-driver"))

	// Call the FindNearest method
	locationRequest := &DriverLocationRequest{
		Location: "test-location",
	}

	// Create a test server
	server := NewServer("localhost:8080", "otlp", nil, log.NewFactory(zapLogger))
	ctx := context.Background()

	// Define a mock Redis implementation for testing
	mockRedis := &Redis{
		// Implement the necessary methods for testing
		tracer: trace.NewNoopTracerProvider().Tracer("test-tracer"),
		logger: server.logger,
	}

	// Assign the mock Redis to the server
	server.redis = mockRedis

	response, err := server.FindNearest(ctx, locationRequest)

	// Assert the expected results
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 10, len(response.Locations))
	// assert.Equal(t, "T723310C", response.Locations[0].DriverID)
	// assert.Equal(t, "222,953", response.Locations[0].Location)
	// Add more assertions as needed
}
