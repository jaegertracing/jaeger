// Copyright (c) 2023 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
)

func TestFindNearest(t *testing.T) {
	logger := zap.NewNop()
	zapLogger := logger.With(zap.String("service", "test-driver"))

	hostPort := "localhost:8080"

	/* lis, err := net.Listen("tcp", hostPort)
	server := grpc.NewServer(grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
	).Serve(lis) */

	// Call the FindNearest method
	locationRequest := &DriverLocationRequest{
		Location: "222,953",
	}

	// Create a test server
	server := NewServer(hostPort, "otlp", nil, log.NewFactory(zapLogger))
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

	// server.redis.FindDriverIDs(ctx, locationRequest.Location)

	// server.redis.GetDriver(ctx, driverId)

	// Assert the expected results
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 10, len(response.Locations))
	// assert.Equal(t, "T723310C", response.Locations[0].DriverID)
	// assert.Equal(t, "222,953", response.Locations[0].Location)
	// Add more assertions as needed
}
