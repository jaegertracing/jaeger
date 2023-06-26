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

	// Call the FindNearest method
	response, err := server.FindNearest(ctx, locationRequest)

	driverIDs := server.redis.FindDriverIDs(ctx, locationRequest.Location)

	retMe := make([]*DriverLocation, len(driverIDs))
	for i, driverID := range driverIDs {
		var drv Driver
		var err error
		for i := 0; i < 3; i++ {
			drv, err = server.redis.GetDriver(ctx, driverID)
			if err == nil {
				break
			}
			server.logger.For(ctx).Error("Retrying GetDriver after error", zap.Int("retry_no", i+1), zap.Error(err))
		}
		if err != nil {
			server.logger.For(ctx).Error("Failed to get driver after 3 attempts", zap.Error(err))
			return
		}
		retMe[i] = &DriverLocation{
			DriverID: drv.DriverID,
			Location: drv.Location,
		}
	}

	// Assert the expected results
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, len(retMe), len(response.Locations))
	// assert.Equal(t, retMe[0].DriverID, response.Locations[0].DriverID)
	// assert.Equal(t, retMe[0].Location, response.Locations[0].Location)
	// Add more assertions as needed
}
