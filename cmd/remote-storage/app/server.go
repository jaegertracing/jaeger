// Copyright (c) 2020 The Jaeger Authors.
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

package app

import (
	"fmt"
	"net"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Server runs a gRPC server
type Server struct {
	logger      *zap.Logger
	healthcheck *healthcheck.HealthCheck
	opts        *Options

	grpcConn   net.Listener
	grpcServer *grpc.Server
	wg         sync.WaitGroup
}

// NewServer creates and initializes Server.
func NewServer(options *Options, storageFactory storage.Factory, tm *tenancy.Manager, logger *zap.Logger, healthcheck *healthcheck.HealthCheck) (*Server, error) {
	handler, err := createGRPCHandler(storageFactory, logger)
	if err != nil {
		return nil, err
	}

	grpcServer, err := createGRPCServer(options, tm, handler, logger)
	if err != nil {
		return nil, err
	}

	return &Server{
		logger:      logger,
		healthcheck: healthcheck,
		opts:        options,
		grpcServer:  grpcServer,
	}, nil
}

func createGRPCHandler(f storage.Factory, logger *zap.Logger) (*shared.GRPCHandler, error) {
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
	if tm.Enabled {
		grpcOpts = append(grpcOpts,
			grpc.StreamInterceptor(tenancy.NewGuardingStreamInterceptor(tm)),
			grpc.UnaryInterceptor(tenancy.NewGuardingUnaryInterceptor(tm)),
		)
	}

	server := grpc.NewServer(grpcOpts...)
	reflection.Register(server)
	handler.Register(server)

	return server, nil
}

// Start gRPC server concurrently
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.opts.GRPCHostPort)
	if err != nil {
		return err
	}
	s.logger.Info("Starting GRPC server", zap.Stringer("addr", listener.Addr()))
	s.grpcConn = listener
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.grpcServer.Serve(s.grpcConn); err != nil {
			s.logger.Error("GRPC server exited", zap.Error(err))
		}
		s.healthcheck.Set(healthcheck.Unavailable)
	}()

	return nil
}

// Close stops http, GRPC servers and closes the port listener.
func (s *Server) Close() error {
	s.grpcServer.Stop()
	s.grpcConn.Close()
	s.opts.TLSGRPC.Close()
	s.wg.Wait()
	return nil
}
