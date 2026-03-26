// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	grpcstorage "github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// Server runs a gRPC server
type Server struct {
	grpcCfg    configgrpc.ServerConfig
	grpcConn   net.Listener
	grpcServer *grpc.Server
	stopped    sync.WaitGroup
	telset     telemetry.Settings
}

// NewServer creates and initializes Server.
func NewServer(
	ctx context.Context,
	grpcCfg configgrpc.ServerConfig,
	ts tracestore.Factory,
	ds depstore.Factory,
	tm *tenancy.Manager,
	telset telemetry.Settings,
) (*Server, error) {
	reader, err := ts.CreateTraceReader()
	if err != nil {
		return nil, err
	}
	writer, err := ts.CreateTraceWriter()
	if err != nil {
		return nil, err
	}
	depReader, err := ds.CreateDependencyReader()
	if err != nil {
		return nil, err
	}

	// This is required because we are using the config to start the server.
	// If the config is created manually (e.g. in tests), the transport might not be set.
	grpcCfg.NetAddr.Transport = confignet.TransportTypeTCP

	v2Handler := grpcstorage.NewHandler(reader, writer, depReader)

	grpcServer, err := createGRPCServer(ctx, grpcCfg, tm, v2Handler, telset)
	if err != nil {
		return nil, err
	}

	return &Server{
		grpcCfg:    grpcCfg,
		grpcServer: grpcServer,
		telset:     telset,
	}, nil
}

func createGRPCServer(
	ctx context.Context,
	cfg configgrpc.ServerConfig,
	tm *tenancy.Manager,
	v2Handler *grpcstorage.Handler,
	telset telemetry.Settings,
) (*grpc.Server, error) {
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		bearertoken.NewUnaryServerInterceptor(),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		bearertoken.NewStreamServerInterceptor(),
	}
	//nolint:contextcheck // The context is handled by the interceptors
	if tm.Enabled {
		unaryInterceptors = append(unaryInterceptors, tenancy.NewGuardingUnaryInterceptor(tm))
		streamInterceptors = append(streamInterceptors, tenancy.NewGuardingStreamInterceptor(tm))
	}

	cfg.NetAddr.Transport = confignet.TransportTypeTCP
	var extensions map[component.ID]component.Component
	if telset.Host != nil {
		extensions = telset.Host.GetExtensions()
	}
	server, err := cfg.ToServer(ctx,
		extensions,
		telset.ToOtelComponent(),
		configgrpc.WithGrpcServerOption(grpc.ChainUnaryInterceptor(unaryInterceptors...)),
		configgrpc.WithGrpcServerOption(grpc.ChainStreamInterceptor(streamInterceptors...)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC server: %w", err)
	}
	healthServer := health.NewServer()
	reflection.Register(server)

	v2Handler.Register(server, healthServer)
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	return server, nil
}

// Start gRPC server concurrently
func (s *Server) Start(ctx context.Context) error {
	var err error
	s.grpcConn, err = s.grpcCfg.NetAddr.Listen(ctx)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port: %w", err)
	}
	s.telset.Logger.Info("Starting GRPC server", zap.Stringer("addr", s.grpcConn.Addr()))
	s.stopped.Go(func() {
		if err := s.grpcServer.Serve(s.grpcConn); err != nil {
			s.telset.Logger.Error("GRPC server exited", zap.Error(err))
			s.telset.ReportStatus(componentstatus.NewFatalErrorEvent(err))
		}
	})

	return nil
}

// Close stops http, GRPC servers and closes the port listener.
func (s *Server) Close() error {
	s.grpcServer.Stop()
	s.stopped.Wait()
	s.telset.ReportStatus(componentstatus.NewEvent(componentstatus.StatusStopped))
	return nil
}

func (s *Server) GRPCAddr() string {
	return s.grpcConn.Addr().String()
}
