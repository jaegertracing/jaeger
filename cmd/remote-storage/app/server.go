// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net"
	"sync"

	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Server runs a gRPC server
type Server struct {
	opts       *Options
	grpcConn   net.Listener
	grpcServer *grpc.Server
	stopped    sync.WaitGroup
	telset     telemetry.Settings
}

// NewServer creates and initializes Server.
func NewServer(options *Options, storageFactory *storage.Factory, tm *tenancy.Manager, telset telemetry.Settings) (*Server, error) {
	handler, err := createGRPCHandler(storageFactory, telset.Logger)
	if err != nil {
		return nil, err
	}

	grpcServer, err := createGRPCServer(options, tm, handler, telset)
	if err != nil {
		return nil, err
	}

	return &Server{
		opts:       options,
		grpcServer: grpcServer,
		telset:     telset,
	}, nil
}

func createGRPCHandler(f *storage.Factory, logger *zap.Logger) (*shared.GRPCHandler, error) {
	reader, err := f.CreateSpanReader()
	if err != nil {
		return nil, err
	}
	writer, err := f.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	depReader, err := f.CreateDependencyReader()
	if err != nil {
		return nil, err
	}

	impl := &shared.GRPCHandlerStorageImpl{
		SpanReader:          func() spanstore.Reader { return reader },
		SpanWriter:          func() spanstore.Writer { return writer },
		DependencyReader:    func() dependencystore.Reader { return depReader },
		StreamingSpanWriter: func() spanstore.Writer { return nil },
	}

	ar, aw := initArchiveStorage(f, logger)
	impl.ArchiveSpanReader = func() spanstore.Reader { return ar }
	impl.ArchiveSpanWriter = func() spanstore.Writer { return aw }

	handler := shared.NewGRPCHandler(impl)
	return handler, nil
}

func createGRPCServer(opts *Options, tm *tenancy.Manager, handler *shared.GRPCHandler, telset telemetry.Settings) (*grpc.Server, error) {
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		bearertoken.NewUnaryServerInterceptor(),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		bearertoken.NewStreamServerInterceptor(),
	}
	if tm.Enabled {
		unaryInterceptors = append(unaryInterceptors, tenancy.NewGuardingUnaryInterceptor(tm))
		streamInterceptors = append(streamInterceptors, tenancy.NewGuardingStreamInterceptor(tm))
	}

	opts.NetAddr.Transport = confignet.TransportTypeTCP
	server, err := opts.ToServer(context.Background(),
		telset.Host,
		telset.ToOtelComponent(),
		configgrpc.WithGrpcServerOption(grpc.ChainUnaryInterceptor(unaryInterceptors...)),
		configgrpc.WithGrpcServerOption(grpc.ChainStreamInterceptor(streamInterceptors...)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC server: %w", err)
	}
	healthServer := health.NewServer()
	reflection.Register(server)
	handler.Register(server, healthServer)

	return server, nil
}

// Start gRPC server concurrently
func (s *Server) Start() error {
	var err error
	s.grpcConn, err = s.opts.NetAddr.Listen(context.Background())
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port: %w", err)
	}
	s.telset.Logger.Info("Starting GRPC server", zap.Stringer("addr", s.grpcConn.Addr()))
	s.stopped.Add(1)
	go func() {
		defer s.stopped.Done()
		if err := s.grpcServer.Serve(s.grpcConn); err != nil {
			s.telset.Logger.Error("GRPC server exited", zap.Error(err))
			s.telset.ReportStatus(componentstatus.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

// Close stops http, GRPC servers and closes the port listener.
func (s *Server) Close() error {
	s.grpcServer.Stop()
	s.stopped.Wait()
	s.telset.ReportStatus(componentstatus.NewEvent(componentstatus.StatusStopped))
	return nil
}

func initArchiveStorage(
	storageFactory *storage.Factory,
	logger *zap.Logger,
) (spanstore.Reader, spanstore.Writer) {
	reader, err := storageFactory.CreateArchiveSpanReader()
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return nil, nil
	}
	writer, err := storageFactory.CreateArchiveSpanWriter()
	if err != nil {
		logger.Error("Cannot init archive storage writer", zap.Error(err))
		return nil, nil
	}

	return reader, writer
}
