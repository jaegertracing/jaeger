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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/soheilhy/cmux"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/query/app/apiv3"
	"github.com/jaegertracing/jaeger/cmd/query/app/internal/api_v3"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/netutils"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/telemetery"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

// Server runs HTTP, Mux and a grpc server
type Server struct {
	querySvc     *querysvc.QueryService
	queryOptions *QueryOptions

	conn          net.Listener
	grpcConn      net.Listener
	httpConn      net.Listener
	cmuxServer    cmux.CMux
	grpcServer    *grpc.Server
	httpServer    *httpServer
	separatePorts bool
	bgFinished    sync.WaitGroup
	telemetery.Setting
}

// NewServer creates and initializes Server
func NewServer(querySvc *querysvc.QueryService,
	metricsQuerySvc querysvc.MetricsQueryService,
	options *QueryOptions,
	tm *tenancy.Manager,
	telset telemetery.Setting,
) (*Server, error) {
	_, httpPort, err := net.SplitHostPort(options.HTTPHostPort)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP server host:port: %w", err)
	}
	_, grpcPort, err := net.SplitHostPort(options.GRPCHostPort)
	if err != nil {
		return nil, fmt.Errorf("invalid gRPC server host:port: %w", err)
	}
	separatePorts := grpcPort != httpPort || grpcPort == "0" || httpPort == "0"

	if (options.TLSHTTP.Enabled || options.TLSGRPC.Enabled) && !separatePorts {
		return nil, errors.New("server with TLS enabled can not use same host ports for gRPC and HTTP.  Use dedicated HTTP and gRPC host ports instead")
	}

	grpcServer, err := createGRPCServer(querySvc, metricsQuerySvc, options, tm, telset)
	if err != nil {
		return nil, err
	}

	httpServer, err := createHTTPServer(querySvc, metricsQuerySvc, options, tm, telset)
	if err != nil {
		return nil, err
	}

	return &Server{
		querySvc:      querySvc,
		queryOptions:  options,
		grpcServer:    grpcServer,
		httpServer:    httpServer,
		separatePorts: separatePorts,
		Setting:       telset,
	}, nil
}

func createGRPCServer(querySvc *querysvc.QueryService, metricsQuerySvc querysvc.MetricsQueryService, options *QueryOptions, tm *tenancy.Manager, telset telemetery.Setting) (*grpc.Server, error) {
	var grpcOpts []grpc.ServerOption

	if options.TLSGRPC.Enabled {
		tlsCfg, err := options.TLSGRPC.Config(telset.Logger)
		if err != nil {
			return nil, err
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

	handler := NewGRPCHandler(querySvc, metricsQuerySvc, GRPCHandlerOptions{
		Logger: telset.Logger,
	})
	healthServer := health.NewServer()

	api_v2.RegisterQueryServiceServer(server, handler)
	metrics.RegisterMetricsQueryServiceServer(server, handler)
	api_v3.RegisterQueryServiceServer(server, &apiv3.Handler{QueryService: querySvc})

	healthServer.SetServingStatus("jaeger.api_v2.QueryService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("jaeger.api_v2.metrics.MetricsQueryService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("jaeger.api_v3.QueryService", grpc_health_v1.HealthCheckResponse_SERVING)

	grpc_health_v1.RegisterHealthServer(server, healthServer)
	return server, nil
}

type httpServer struct {
	*http.Server
	staticHandlerCloser io.Closer
}

var _ io.Closer = (*httpServer)(nil)

func createHTTPServer(
	querySvc *querysvc.QueryService,
	metricsQuerySvc querysvc.MetricsQueryService,
	queryOpts *QueryOptions,
	tm *tenancy.Manager,
	telset telemetery.Setting,
) (*httpServer, error) {
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.Logger(telset.Logger),
		HandlerOptions.Tracer(telset.TracerProvider),
		HandlerOptions.MetricsQueryService(metricsQuerySvc),
	}

	apiHandler := NewAPIHandler(
		querySvc,
		tm,
		apiHandlerOptions...)
	r := NewRouter()
	if queryOpts.BasePath != "/" {
		r = r.PathPrefix(queryOpts.BasePath).Subrouter()
	}

	(&apiv3.HTTPGateway{
		QueryService: querySvc,
		TenancyMgr:   tm,
		Logger:       telset.Logger,
		Tracer:       telset.TracerProvider,
	}).RegisterRoutes(r)

	apiHandler.RegisterRoutes(r)
	var handler http.Handler = r
	handler = additionalHeadersHandler(handler, queryOpts.AdditionalHeaders)
	if queryOpts.BearerTokenPropagation {
		handler = bearertoken.PropagationHandler(telset.Logger, handler)
	}
	handler = handlers.CompressHandler(handler)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(telset.Logger, true)

	errorLog, _ := zap.NewStdLogAt(telset.Logger, zapcore.ErrorLevel)
	server := &httpServer{
		Server: &http.Server{
			Handler:           recoveryHandler(handler),
			ErrorLog:          errorLog,
			ReadHeaderTimeout: 2 * time.Second,
		},
	}

	if queryOpts.TLSHTTP.Enabled {
		tlsCfg, err := queryOpts.TLSHTTP.Config(telset.Logger) // This checks if the certificates are correctly provided
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsCfg
	}

	server.staticHandlerCloser = RegisterStaticHandler(r, telset.Logger, queryOpts, querySvc.GetCapabilities())

	return server, nil
}

func (hS httpServer) Close() error {
	var errs []error
	errs = append(errs, hS.Server.Close())
	errs = append(errs, hS.staticHandlerCloser.Close())
	return errors.Join(errs...)
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
		s.Logger.Info(
			"Query server started",
			zap.String("http_addr", s.HTTPAddr()),
			zap.String("grpc_addr", s.GRPCAddr()),
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

	s.Logger.Info(
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
		return fmt.Errorf("query server failed to initialize listener: %w", err)
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

	s.bgFinished.Add(1)
	go func() {
		defer s.bgFinished.Done()
		s.Logger.Info("Starting HTTP server", zap.Int("port", httpPort), zap.String("addr", s.queryOptions.HTTPHostPort))
		var err error
		if s.queryOptions.TLSHTTP.Enabled {
			err = s.httpServer.ServeTLS(s.httpConn, "", "")
		} else {
			err = s.httpServer.Serve(s.httpConn)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, cmux.ErrListenerClosed) && !errors.Is(err, cmux.ErrServerClosed) {
			s.Logger.Error("Could not start HTTP server", zap.Error(err))
			s.ReportStatus(component.NewFatalErrorEvent(err))
			return
		}
		s.Logger.Info("HTTP server stopped", zap.Int("port", httpPort), zap.String("addr", s.queryOptions.HTTPHostPort))
	}()

	// Start GRPC server concurrently
	s.bgFinished.Add(1)
	go func() {
		defer s.bgFinished.Done()
		s.Logger.Info("Starting GRPC server", zap.Int("port", grpcPort), zap.String("addr", s.queryOptions.GRPCHostPort))

		err := s.grpcServer.Serve(s.grpcConn)
		if err != nil && !errors.Is(err, cmux.ErrListenerClosed) && !errors.Is(err, cmux.ErrServerClosed) {
			s.Logger.Error("Could not start GRPC server", zap.Error(err))
			s.ReportStatus(component.NewFatalErrorEvent(err))
			return
		}
		s.Logger.Info("GRPC server stopped", zap.Int("port", grpcPort), zap.String("addr", s.queryOptions.GRPCHostPort))
	}()

	// Start cmux server concurrently.
	if !s.separatePorts {
		s.bgFinished.Add(1)
		go func() {
			defer s.bgFinished.Done()
			s.Logger.Info("Starting CMUX server", zap.Int("port", tcpPort), zap.String("addr", s.queryOptions.HTTPHostPort))

			err := cmuxServer.Serve()
			// TODO: find a way to avoid string comparison. Even though cmux has ErrServerClosed, it's not returned here.
			if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				s.Logger.Error("Could not start multiplexed server", zap.Error(err))
				s.ReportStatus(component.NewFatalErrorEvent(err))
				return
			}
			s.Logger.Info("CMUX server stopped", zap.Int("port", tcpPort), zap.String("addr", s.queryOptions.HTTPHostPort))
		}()
	}
	return nil
}

func (s *Server) HTTPAddr() string {
	return s.httpConn.Addr().String()
}

func (s *Server) GRPCAddr() string {
	return s.grpcConn.Addr().String()
}

// Close stops HTTP, GRPC servers and closes the port listener.
func (s *Server) Close() error {
	errs := []error{
		s.queryOptions.TLSGRPC.Close(),
		s.queryOptions.TLSHTTP.Close(),
	}

	s.Logger.Info("Closing HTTP server")
	if err := s.httpServer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close HTTP server: %w", err))
	}

	s.Logger.Info("Stopping gRPC server")
	s.grpcServer.Stop()

	if !s.separatePorts {
		s.Logger.Info("Closing CMux server")
		s.cmuxServer.Close()
	}
	s.bgFinished.Wait()

	s.Logger.Info("Server stopped")
	return errors.Join(errs...)
}
