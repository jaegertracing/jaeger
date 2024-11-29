// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// GRPCServerParams to construct a new Jaeger Collector gRPC Server
type GRPCServerParams struct {
	TLSConfig               *configtls.ServerConfig
	HostPort                string
	Handler                 *handler.GRPCHandler
	SamplingProvider        samplingstrategy.Provider
	Logger                  *zap.Logger
	OnError                 func(error)
	MaxReceiveMessageLength int
	MaxConnectionAge        time.Duration
	MaxConnectionAgeGrace   time.Duration

	// Set by the server to indicate the actual host:port of the server.
	HostPortActual string
}

// StartGRPCServer based on the given parameters
func StartGRPCServer(params *GRPCServerParams) (*grpc.Server, error) {
	var server *grpc.Server
	var grpcOpts []grpc.ServerOption

	if params.MaxReceiveMessageLength > 0 {
		grpcOpts = append(grpcOpts, grpc.MaxRecvMsgSize(params.MaxReceiveMessageLength))
	}
	grpcOpts = append(grpcOpts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge:      params.MaxConnectionAge,
		MaxConnectionAgeGrace: params.MaxConnectionAgeGrace,
	}))

	if params.TLSConfig != nil {
		// user requested a server with TLS, setup creds
		tlsCfg, err := params.TLSConfig.LoadTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}

		creds := credentials.NewTLS(tlsCfg)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

	server = grpc.NewServer(grpcOpts...)
	reflection.Register(server)

	listener, err := net.Listen("tcp", params.HostPort)
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
	api_v2.RegisterSamplingManagerServer(server, sampling.NewGRPCHandler(params.SamplingProvider))

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
