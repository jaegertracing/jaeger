// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	samplinggrpc "github.com/jaegertracing/jaeger/internal/sampling/grpc"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// GRPCServerParams to construct a new Jaeger Collector gRPC Server
type GRPCServerParams struct {
	configgrpc.ServerConfig
	Handler          *handler.GRPCHandler
	SamplingProvider samplingstrategy.Provider
	Logger           *zap.Logger
	OnError          func(error)

	// Set by the server to indicate the actual host:port of the server.
	HostPortActual string
}

// StartGRPCServer based on the given parameters
func StartGRPCServer(params *GRPCServerParams) (*grpc.Server, error) {
	var server *grpc.Server
	var grpcOpts []configgrpc.ToServerOption
	params.NetAddr.Transport = confignet.TransportTypeTCP
	server, err := params.ToServer(context.Background(), nil,
		telemetry.NoopSettings().ToOtelComponent(),
		grpcOpts...)
	if err != nil {
		return nil, err
	}
	reflection.Register(server)

	listener, err := params.NetAddr.Listen(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to listen on gRPC port: %w", err)
	}
	params.HostPortActual = listener.Addr().String()

	if err := serveGRPC(server, listener, params); err != nil {
		return nil, err
	}

	return server, nil
}

func serveGRPC(server *grpc.Server, listener net.Listener, params *GRPCServerParams) error {
	healthServer := health.NewServer()

	api_v2.RegisterCollectorServiceServer(server, params.Handler)
	api_v2.RegisterSamplingManagerServer(server, samplinggrpc.NewHandler(params.SamplingProvider))

	healthServer.SetServingStatus("jaeger.api_v2.CollectorService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("jaeger.api_v2.SamplingManager", grpc_health_v1.HealthCheckResponse_SERVING)

	grpc_health_v1.RegisterHealthServer(server, healthServer)

	params.Logger.Info("Starting jaeger-collector gRPC server", zap.String("grpc.host-port", params.HostPortActual))
	go func() {
		if err := server.Serve(listener); err != nil {
			params.Logger.Error("Could not launch gRPC service", zap.Error(err))
			if params.OnError != nil {
				params.OnError(err)
			}
		}
	}()

	return nil
}
