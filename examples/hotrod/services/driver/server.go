// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"encoding/json"
	"net"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// Server implements jaeger-demo-frontend service
type Server struct {
	hostPort string
	logger   log.Factory
	redis    *Redis
	server   *grpc.Server
}

var _ DriverServiceServer = (*Server)(nil)

// NewServer creates a new driver.Server
func NewServer(hostPort string, otelExporter string, metricsFactory metrics.Factory, logger log.Factory) *Server {
	tracerProvider := tracing.InitOTEL("driver", otelExporter, metricsFactory, logger)
	server := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracerProvider))),
	)
	return &Server{
		hostPort: hostPort,
		logger:   logger,
		server:   server,
		redis:    newRedis(otelExporter, metricsFactory, logger),
	}
}

// Run starts the Driver server
func (s *Server) Run() error {
	lis, err := net.Listen("tcp", s.hostPort)
	if err != nil {
		s.logger.Bg().Fatal("Unable to create http listener", zap.Error(err))
	}
	RegisterDriverServiceServer(s.server, s)
	s.logger.Bg().Info("Starting", zap.String("address", s.hostPort), zap.String("type", "gRPC"))
	err = s.server.Serve(lis)
	if err != nil {
		s.logger.Bg().Fatal("Unable to start gRPC server", zap.Error(err))
	}
	return err
}

// FindNearest implements gRPC driver interface
func (s *Server) FindNearest(ctx context.Context, location *DriverLocationRequest) (*DriverLocationResponse, error) {
	s.logger.For(ctx).Info("Searching for nearby drivers", zap.String("location", location.Location))
	driverIDs := s.redis.FindDriverIDs(ctx, location.Location)

	locations := make([]*DriverLocation, len(driverIDs))
	for i, driverID := range driverIDs {
		var drv Driver
		var err error
		for i := 0; i < 3; i++ {
			drv, err = s.redis.GetDriver(ctx, driverID)
			if err == nil {
				break
			}
			s.logger.For(ctx).Error("Retrying GetDriver after error", zap.Int("retry_no", i+1), zap.Error(err))
		}
		if err != nil {
			s.logger.For(ctx).Error("Failed to get driver after 3 attempts", zap.Error(err))
			return nil, err
		}
		locations[i] = &DriverLocation{
			DriverID: drv.DriverID,
			Location: drv.Location,
		}
	}
	s.logger.For(ctx).Info(
		"Search successful",
		zap.Int("driver_count", len(locations)),
		zap.String("locations", toJSON(locations)),
	)
	return &DriverLocationResponse{Locations: locations}, nil
}

func toJSON(v any) string {
	str, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(str)
}
