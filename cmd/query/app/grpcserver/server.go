// Copyright (c) 2019 The Jaeger Authors.
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

package grpcserver

import (
	"fmt"
	"net"
	"net/http"

	"github.com/opentracing/opentracing-go"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

const (
	grpcListener = "grpc"
	httpListener = "http"
)

// Builder provides the configuration used to build a GrpcServer
type Builder struct {
	Svc             *flags.Service
	RecoveryHandler http.Handler
	QuerySvc        querysvc.QueryService
	Logger          *zap.Logger
	Tracer          opentracing.Tracer
	QueryOptions    *app.QueryOptions
}

// GrpcServer runs HTTP, Mux and a grpc server
type GrpcServer struct {
	svc        *flags.Service
	grpcServer *grpc.Server

	listeners  map[string]net.Listener
	httpServer *http.Server
	muxServer  cmux.CMux
	logger     *zap.Logger
	listenPort int
}

func (g *Builder) createHandler() *app.GRPCHandler {
	return app.NewGRPCHandler(g.QuerySvc, g.Logger, g.Tracer)
}

// Build a GrpcServer
func (g *Builder) Build() *GrpcServer {

	// Prepare cmux conn.
	conn, err := net.Listen("tcp", fmt.Sprintf(":%d", g.QueryOptions.Port))
	if err != nil {
		g.Logger.Fatal("Could not start listener", zap.Error(err))
	}
	// Create cmux server.
	// cmux will reverse-proxy between HTTP and GRPC backends.
	cmuxServer := cmux.New(conn)

	// Add GRPC and HTTP listeners.
	listeners := map[string]net.Listener{
		grpcListener: cmuxServer.Match(
			cmux.HTTP2HeaderField("content-type", "application/grpc"),
			cmux.HTTP2HeaderField("content-type", "application/grpc+proto")),

		httpListener: cmuxServer.Match(cmux.Any()),
	}

	return &GrpcServer{
		logger:     g.Logger,
		grpcServer: grpc.NewServer(),
		muxServer:  cmuxServer,
		listeners:  listeners,
		httpServer: &http.Server{
			Handler: g.RecoveryHandler,
		},
	}
}

// Start http, GRPC and cmux servers concurrently
func (s *GrpcServer) Start() {

	logger := s.logger // shortcut

	go func() {
		logger.Info("Starting HTTP server", zap.Int("port", s.listenPort))
		if err := s.httpServer.Serve(s.listeners[httpListener]); err != nil {
			logger.Fatal("Could not start HTTP server", zap.Error(err))
		}
		s.svc.HC().Set(healthcheck.Unavailable)
	}()

	// Start GRPC server concurrently
	go func() {
		logger.Info("Starting GRPC server", zap.Int("port", s.listenPort))
		if err := s.grpcServer.Serve(s.listeners[grpcListener]); err != nil {
			logger.Fatal("Could not start GRPC server", zap.Error(err))
		}
		s.svc.HC().Set(healthcheck.Unavailable)
	}()

	// Start cmux server concurrently.
	go func() {
		if err := s.muxServer.Serve(); err != nil {
			logger.Fatal("Could not start multiplexed server", zap.Error(err))
		}
		s.svc.HC().Set(healthcheck.Unavailable)
	}()

}
