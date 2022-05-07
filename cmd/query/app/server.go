// Copyright (c) 2019,2020 The Jaeger Authors.
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
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/opentracing/opentracing-go"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/query/app/apiv3"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/netutils"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
)

// Server runs HTTP, Mux and a grpc server
type Server struct {
	logger       *zap.Logger
	querySvc     *querysvc.QueryService
	queryOptions *QueryOptions

	tracer opentracing.Tracer // TODO make part of flags.Service

	conn               net.Listener
	grpcConn           net.Listener
	httpConn           net.Listener
	cmuxServer         cmux.CMux
	grpcServer         *grpc.Server
	httpServer         *http.Server
	separatePorts      bool
	unavailableChannel chan healthcheck.Status
	grpcGatewayCancel  context.CancelFunc
}

// NewServer creates and initializes Server
func NewServer(logger *zap.Logger, querySvc *querysvc.QueryService, metricsQuerySvc querysvc.MetricsQueryService, options *QueryOptions, tracer opentracing.Tracer) (*Server, error) {

	_, httpPort, err := net.SplitHostPort(options.HTTPHostPort)
	if err != nil {
		return nil, err
	}
	_, grpcPort, err := net.SplitHostPort(options.GRPCHostPort)
	if err != nil {
		return nil, err
	}

	if (options.TLSHTTP.Enabled || options.TLSGRPC.Enabled) && (grpcPort == httpPort) {
		return nil, errors.New("server with TLS enabled can not use same host ports for gRPC and HTTP.  Use dedicated HTTP and gRPC host ports instead")
	}

	grpcServer, err := createGRPCServer(querySvc, metricsQuerySvc, options, logger, tracer)
	if err != nil {
		return nil, err
	}

	httpServer, closeGRPCGateway, err := createHTTPServer(querySvc, metricsQuerySvc, options, tracer, logger)
	if err != nil {
		return nil, err
	}

	return &Server{
		logger:             logger,
		querySvc:           querySvc,
		queryOptions:       options,
		tracer:             tracer,
		grpcServer:         grpcServer,
		httpServer:         httpServer,
		separatePorts:      grpcPort != httpPort,
		unavailableChannel: make(chan healthcheck.Status),
		grpcGatewayCancel:  closeGRPCGateway,
	}, nil
}

// HealthCheckStatus returns health check status channel a client can subscribe to
func (s Server) HealthCheckStatus() chan healthcheck.Status {
	return s.unavailableChannel
}

func createGRPCServer(querySvc *querysvc.QueryService, metricsQuerySvc querysvc.MetricsQueryService, options *QueryOptions, logger *zap.Logger, tracer opentracing.Tracer) (*grpc.Server, error) {
	var grpcOpts []grpc.ServerOption

	if options.TLSGRPC.Enabled {
		tlsCfg, err := options.TLSGRPC.Config(logger)
		if err != nil {
			return nil, err
		}

		creds := credentials.NewTLS(tlsCfg)

		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

	server := grpc.NewServer(grpcOpts...)
	reflection.Register(server)

	handler := &GRPCHandler{
		queryService:        querySvc,
		metricsQueryService: metricsQuerySvc,
		logger:              logger,
		tracer:              tracer,
		nowFn:               time.Now,
	}
	api_v2.RegisterQueryServiceServer(server, handler)
	metrics.RegisterMetricsQueryServiceServer(server, handler)
	api_v3.RegisterQueryServiceServer(server, &apiv3.Handler{QueryService: querySvc})
	return server, nil
}

func createHTTPServer(querySvc *querysvc.QueryService, metricsQuerySvc querysvc.MetricsQueryService, queryOpts *QueryOptions, tracer opentracing.Tracer, logger *zap.Logger) (*http.Server, context.CancelFunc, error) {
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.Logger(logger),
		HandlerOptions.Tracer(tracer),
		HandlerOptions.MetricsQueryService(metricsQuerySvc),
	}

	apiHandler := NewAPIHandler(
		querySvc,
		apiHandlerOptions...)
	r := NewRouter()
	if queryOpts.BasePath != "/" {
		r = r.PathPrefix(queryOpts.BasePath).Subrouter()
	}

	ctx, closeGRPCGateway := context.WithCancel(context.Background())
	if err := apiv3.RegisterGRPCGateway(ctx, logger, r, queryOpts.BasePath, queryOpts.GRPCHostPort, queryOpts.TLSGRPC); err != nil {
		closeGRPCGateway()
		return nil, nil, err
	}

	apiHandler.RegisterRoutes(r)
	RegisterStaticHandler(r, logger, queryOpts)
	var handler http.Handler = r
	handler = additionalHeadersHandler(handler, queryOpts.AdditionalHeaders)
	if queryOpts.BearerTokenPropagation {
		handler = bearertoken.PropagationHandler(logger, handler)
	}
	handler = handlers.CompressHandler(handler)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

	errorLog, _ := zap.NewStdLogAt(logger, zapcore.ErrorLevel)
	server := &http.Server{
		Handler:  recoveryHandler(handler),
		ErrorLog: errorLog,
	}

	if queryOpts.TLSHTTP.Enabled {
		tlsCfg, err := queryOpts.TLSHTTP.Config(logger) // This checks if the certificates are correctly provided
		if err != nil {
			closeGRPCGateway()
			return nil, nil, err
		}
		server.TLSConfig = tlsCfg

	}
	return server, closeGRPCGateway, nil
}

// initListener initialises listeners of the server
func (s *Server) initListener() (cmux.CMux, error) {
	if s.separatePorts { // use separate ports and listeners each for gRPC and HTTP requests
		var err error
		s.grpcConn, err = net.Listen("tcp", s.queryOptions.GRPCHostPort)
		if err != nil {
			return nil, err
		}

		s.httpConn, err = net.Listen("tcp", s.queryOptions.HTTPHostPort)
		if err != nil {
			return nil, err
		}
		s.logger.Info(
			"Query server started",
			zap.String("http_addr", s.httpConn.Addr().String()),
			zap.String("grpc_addr", s.grpcConn.Addr().String()),
		)
		return nil, nil
	}

	//  old behavior using cmux
	conn, err := net.Listen("tcp", s.queryOptions.HTTPHostPort)
	if err != nil {
		return nil, err
	}

	s.conn = conn

	var tcpPort int
	if port, err := netutils.GetPort(s.conn.Addr()); err == nil {
		tcpPort = port
	}

	s.logger.Info(
		"Query server started",
		zap.Int("port", tcpPort),
		zap.String("addr", s.queryOptions.HTTPHostPort))

	// cmux server acts as a reverse-proxy between HTTP and GRPC backends.
	cmuxServer := cmux.New(s.conn)

	s.grpcConn = cmuxServer.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"),
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc+proto"),
	)
	s.httpConn = cmuxServer.Match(cmux.Any())

	return cmuxServer, nil
}

// Start http, GRPC and cmux servers concurrently
func (s *Server) Start() error {
	cmuxServer, err := s.initListener()
	if err != nil {
		return err
	}
	s.cmuxServer = cmuxServer

	var tcpPort int
	if !s.separatePorts {
		if port, err := netutils.GetPort(s.conn.Addr()); err == nil {
			tcpPort = port
		}
	}

	var httpPort int
	if port, err := netutils.GetPort(s.httpConn.Addr()); err == nil {
		httpPort = port
	}

	var grpcPort int
	if port, err := netutils.GetPort(s.grpcConn.Addr()); err == nil {
		grpcPort = port
	}

	go func() {
		s.logger.Info("Starting HTTP server", zap.Int("port", httpPort), zap.String("addr", s.queryOptions.HTTPHostPort))
		var err error
		if s.queryOptions.TLSHTTP.Enabled {
			err = s.httpServer.ServeTLS(s.httpConn, "", "")
		} else {
			err = s.httpServer.Serve(s.httpConn)
		}
		switch err {
		case nil, http.ErrServerClosed, cmux.ErrListenerClosed, cmux.ErrServerClosed:
			// normal exit, nothing to do
		default:
			s.logger.Error("Could not start HTTP server", zap.Error(err))
		}

		s.unavailableChannel <- healthcheck.Unavailable
	}()

	// Start GRPC server concurrently
	go func() {
		s.logger.Info("Starting GRPC server", zap.Int("port", grpcPort), zap.String("addr", s.queryOptions.GRPCHostPort))

		if err := s.grpcServer.Serve(s.grpcConn); err != nil {
			s.logger.Error("Could not start GRPC server", zap.Error(err))
		}
		s.unavailableChannel <- healthcheck.Unavailable
	}()

	// Start cmux server concurrently.
	if !s.separatePorts {
		go func() {
			s.logger.Info("Starting CMUX server", zap.Int("port", tcpPort), zap.String("addr", s.queryOptions.HTTPHostPort))

			err := cmuxServer.Serve()
			// TODO: find a way to avoid string comparison. Even though cmux has ErrServerClosed, it's not returned here.
			if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				s.logger.Error("Could not start multiplexed server", zap.Error(err))
			}
			s.unavailableChannel <- healthcheck.Unavailable
		}()
	}

	return nil
}

// Close stops http, GRPC servers and closes the port listener.
func (s *Server) Close() error {
	s.grpcGatewayCancel()
	s.queryOptions.TLSGRPC.Close()
	s.queryOptions.TLSHTTP.Close()
	s.grpcServer.Stop()
	s.httpServer.Close()
	if s.separatePorts {
		s.httpConn.Close()
		s.grpcConn.Close()
	} else {
		s.cmuxServer.Close()
		s.conn.Close()
	}
	return nil
}
