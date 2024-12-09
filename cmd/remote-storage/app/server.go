// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"net"
	"sync"

	"go.opentelemetry.io/collector/component/componentstatus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	DEFAULT_MAX_BUCKET_SIZE = 1
)

// Server runs a gRPC server
type Server struct {
	opts *Options

	grpcConn   net.Listener
	grpcServer *grpc.Server
	wg         sync.WaitGroup
	telset     telemetry.Settings
}

// NewServer creates and initializes Server.
func NewServer(options *Options, storageFactory storage.BaseFactory, tm *tenancy.Manager, telset telemetry.Settings, samplingStoreFactory storage.SamplingStoreFactory) (*Server, error) {
	handler, err := createGRPCHandler(storageFactory, samplingStoreFactory, telset.Logger)
	if err != nil {
		return nil, err
	}

	grpcServer, err := createGRPCServer(options, tm, handler, telset.Logger)
	if err != nil {
		return nil, err
	}

	return &Server{
		opts:       options,
		grpcServer: grpcServer,
		telset:     telset,
	}, nil
}

func createGRPCHandler(f storage.BaseFactory, samplingStoreFactory storage.SamplingStoreFactory, logger *zap.Logger) (*shared.GRPCHandler, error) {
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
	// TODO: Update this to use bucket size from config
	samplingStore, err := samplingStoreFactory.CreateSamplingStore(DEFAULT_MAX_BUCKET_SIZE)
	if err != nil {
		return nil, err
	}

	impl := &shared.GRPCHandlerStorageImpl{
		SpanReader:          func() spanstore.Reader { return reader },
		SpanWriter:          func() spanstore.Writer { return writer },
		DependencyReader:    func() dependencystore.Reader { return depReader },
		StreamingSpanWriter: func() spanstore.Writer { return nil },
		SamplingStore:       func() samplingstore.Store { return samplingStore },
	}

	// borrow code from Query service for archive storage
	qOpts := &querysvc.QueryServiceOptions{}
	// when archive storage not initialized (returns false), the reader/writer will be nil
	_ = qOpts.InitArchiveStorage(f, logger)
	impl.ArchiveSpanReader = func() spanstore.Reader { return qOpts.ArchiveSpanReader }
	impl.ArchiveSpanWriter = func() spanstore.Writer { return qOpts.ArchiveSpanWriter }

	handler := shared.NewGRPCHandler(impl)
	return handler, nil
}

func createGRPCServer(opts *Options, tm *tenancy.Manager, handler *shared.GRPCHandler, logger *zap.Logger) (*grpc.Server, error) {
	var grpcOpts []grpc.ServerOption

	if opts.TLSGRPC.Enabled {
		tlsCfg, err := opts.TLSGRPC.Config(logger)
		if err != nil {
			return nil, fmt.Errorf("invalid TLS config: %w", err)
		}
		creds := credentials.NewTLS(tlsCfg)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

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

	grpcOpts = append(grpcOpts,
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	server := grpc.NewServer(grpcOpts...)
	healthServer := health.NewServer()
	reflection.Register(server)
	handler.Register(server, healthServer)

	return server, nil
}

// Start gRPC server concurrently
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.opts.GRPCHostPort)
	if err != nil {
		return err
	}
	s.telset.Logger.Info("Starting GRPC server", zap.Stringer("addr", listener.Addr()))
	s.grpcConn = listener
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
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
	s.grpcConn.Close()
	s.opts.TLSGRPC.Close()
	s.wg.Wait()
	s.telset.ReportStatus(componentstatus.NewEvent(componentstatus.StatusStopped))
	return nil
}
