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

package app

import (
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/opentracing/opentracing-go"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// Server runs HTTP, Mux and a grpc server
type Server struct {
	svc          *flags.Service
	grpcServer   *grpc.Server
	grpcListener net.Listener
	httpListener net.Listener
	httpServer   *http.Server
	muxServer    cmux.CMux
	querySvc     querysvc.QueryService
	tracker      opentracing.Tracer
	listenPort   int
}

func createHandler(querySvc querysvc.QueryService, logger *zap.Logger, tracker opentracing.Tracer) *GRPCHandler {
	return NewGRPCHandler(querySvc, logger, tracker)
}

func createHTTPServer(router *mux.Router, logger *zap.Logger) *http.Server {
	compressHandler := handlers.CompressHandler(router)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

	return &http.Server{
		Handler: recoveryHandler(compressHandler),
	}
}

// NewServer creates and initializes Server
func NewServer(svc *flags.Service, router *mux.Router, querySvc querysvc.QueryService, tracker opentracing.Tracer, port int) (*Server, error) {

	// Prepare cmux conn.
	conn, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	// Create cmux server.
	// cmux will reverse-proxy between HTTP and GRPC backends.
	cmuxServer := cmux.New(conn)
	return &Server{
		svc:        svc,
		grpcServer: grpc.NewServer(),
		grpcListener: cmuxServer.Match(
			cmux.HTTP2HeaderField("content-type", "application/grpc"),
			cmux.HTTP2HeaderField("content-type", "application/grpc+proto")),
		httpListener: cmuxServer.Match(cmux.Any()),
		httpServer:   createHTTPServer(router, svc.Logger),
		muxServer:    cmuxServer,
		querySvc:     querysvc.QueryService{},
		tracker:      tracker,
		listenPort:   port,
	}, nil
}

// Start http, GRPC and cmux servers concurrently
func (s *Server) Start() {
	// Create handler
	h := createHandler(s.querySvc, s.svc.Logger, s.tracker)
	api_v2.RegisterQueryServiceServer(s.grpcServer, h)

	go func() {
		s.svc.Logger.Info("Starting HTTP server", zap.Int("port", s.listenPort))
		if err := s.httpServer.Serve(s.httpListener); err != nil {
			s.svc.Logger.Error("Could not start HTTP server", zap.Error(err))
		}
		s.svc.HC().Set(healthcheck.Unavailable)
	}()

	// Start GRPC server concurrently
	go func() {
		s.svc.Logger.Info("Starting GRPC server", zap.Int("port", s.listenPort))
		if err := s.grpcServer.Serve(s.grpcListener); err != nil {
			s.svc.Logger.Error("Could not start GRPC server", zap.Error(err))
		}
		s.svc.HC().Set(healthcheck.Unavailable)
	}()

	// Start cmux server concurrently.
	go func() {
		if err := s.muxServer.Serve(); err != nil {
			s.svc.Logger.Error("Could not start multiplexed server", zap.Error(err))
		}
		s.svc.HC().Set(healthcheck.Unavailable)
	}()

}
