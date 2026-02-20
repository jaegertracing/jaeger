// Copyright (c) 2019,2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp/xconfighttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/apiv3"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/recoveryhandler"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// Server runs HTTP, Mux and a grpc server
type Server struct {
	queryOptions *QueryOptions
	grpcConn     net.Listener
	httpConn     net.Listener
	grpcServer   *grpc.Server
	httpServer   *httpServer
	bgFinished   sync.WaitGroup
	telset       telemetry.Settings
}

// NewServer creates and initializes Server
func NewServer(
	ctx context.Context,
	querySvc *querysvc.QueryService,
	metricsQuerySvc metricstore.Reader,
	options *QueryOptions,
	tm *tenancy.Manager,
	telset telemetry.Settings,
) (*Server, error) {
	_, httpPort, err := net.SplitHostPort(options.HTTP.NetAddr.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP server host:port: %w", err)
	}
	_, grpcPort, err := net.SplitHostPort(options.GRPC.NetAddr.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid gRPC server host:port: %w", err)
	}
	separatePorts := grpcPort != httpPort || grpcPort == "0" || httpPort == "0"

	if (options.HTTP.TLS.HasValue() || options.GRPC.TLS.HasValue()) && !separatePorts {
		return nil, errors.New("server with TLS enabled can not use same host ports for gRPC and HTTP.  Use dedicated HTTP and gRPC host ports instead")
	}

	grpcServer, err := createGRPCServer(ctx, options, tm, telset)
	if err != nil {
		return nil, err
	}
	registerGRPCHandlers(grpcServer, querySvc, telset)
	httpServer, err := createHTTPServer(ctx, querySvc, metricsQuerySvc, options, tm, telset)
	if err != nil {
		return nil, err
	}

	return &Server{
		queryOptions: options,
		grpcServer:   grpcServer,
		httpServer:   httpServer,
		telset:       telset,
	}, nil
}

func registerGRPCHandlers(
	server *grpc.Server,
	querySvc *querysvc.QueryService,
	telset telemetry.Settings,
) {
	reflection.Register(server)
	handler := NewGRPCHandler(querySvc, GRPCHandlerOptions{Logger: telset.Logger})
	healthServer := health.NewServer()

	api_v2.RegisterQueryServiceServer(server, handler)
	api_v3.RegisterQueryServiceServer(server, &apiv3.Handler{QueryService: querySvc})

	healthServer.SetServingStatus("jaeger.api_v2.QueryService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("jaeger.api_v2.metrics.MetricsQueryService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("jaeger.api_v3.QueryService", grpc_health_v1.HealthCheckResponse_SERVING)

	grpc_health_v1.RegisterHealthServer(server, healthServer)
}

func createGRPCServer(
	ctx context.Context,
	options *QueryOptions,
	tm *tenancy.Manager,
	telset telemetry.Settings,
) (*grpc.Server, error) {
	var grpcOpts []configgrpc.ToServerOption
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

	grpcOpts = append(grpcOpts,
		configgrpc.WithGrpcServerOption(grpc.ChainUnaryInterceptor(unaryInterceptors...)),
		configgrpc.WithGrpcServerOption(grpc.ChainStreamInterceptor(streamInterceptors...)),
	)
	var extensions map[component.ID]component.Component
	if telset.Host != nil {
		extensions = telset.Host.GetExtensions()
	}
	return options.GRPC.ToServer(
		ctx,
		extensions,
		component.TelemetrySettings{
			Logger:         telset.Logger,
			TracerProvider: telset.TracerProvider,
			MeterProvider:  telset.MeterProvider,
		},
		grpcOpts...)
}

type httpServer struct {
	*http.Server
	staticHandlerCloser io.Closer
}

var _ io.Closer = (*httpServer)(nil)

func initRouter(
	querySvc *querysvc.QueryService,
	metricsQuerySvc metricstore.Reader,
	queryOpts *QueryOptions,
	tenancyMgr *tenancy.Manager,
	telset telemetry.Settings,
) (http.Handler, io.Closer) {
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.Logger(telset.Logger),
		HandlerOptions.Tracer(telset.TracerProvider),
		HandlerOptions.MetricsQueryService(metricsQuerySvc),
		HandlerOptions.BasePath(queryOpts.BasePath),
	}

	apiHandler := NewAPIHandler(
		querySvc,
		apiHandlerOptions...)
	r := http.NewServeMux()

	(&apiv3.HTTPGateway{
		QueryService: querySvc,
		Logger:       telset.Logger,
		Tracer:       telset.TracerProvider,
		BasePath:     queryOpts.BasePath,
	}).RegisterRoutes(r)

	apiHandler.RegisterRoutes(r)

	// Register a 404 handler for unmatched /api routes before the static catch-all handler.
	// This prevents the static handler from serving index.html for non-existent API endpoints.
	apiNotFoundPattern := "/api/"
	if queryOpts.BasePath != "" && queryOpts.BasePath != "/" {
		apiNotFoundPattern = queryOpts.BasePath + apiNotFoundPattern
	}
	r.HandleFunc(apiNotFoundPattern, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "404 page not found", http.StatusNotFound)
	})

	staticHandlerCloser := RegisterStaticHandler(r, telset.Logger, queryOpts, querySvc.GetCapabilities())

	var handler http.Handler = r
	if queryOpts.BearerTokenPropagation {
		handler = bearertoken.PropagationHandler(telset.Logger, handler)
	}
	if tenancyMgr.Enabled {
		handler = tenancy.ExtractTenantHTTPHandler(tenancyMgr, handler)
	}
	handler = traceResponseHandler(handler)
	return handler, staticHandlerCloser
}

func createHTTPServer(
	ctx context.Context,
	querySvc *querysvc.QueryService,
	metricsQuerySvc metricstore.Reader,
	queryOpts *QueryOptions,
	tm *tenancy.Manager,
	telset telemetry.Settings,
) (*httpServer, error) {
	handler, staticHandlerCloser := initRouter(querySvc, metricsQuerySvc, queryOpts, tm, telset)
	handler = recoveryhandler.NewRecoveryHandler(telset.Logger, true)(handler)
	var extensions map[component.ID]component.Component
	if telset.Host != nil {
		extensions = telset.Host.GetExtensions()
	}
	hs, err := queryOpts.HTTP.ToServer(
		ctx,
		extensions,
		component.TelemetrySettings{
			Logger:         telset.Logger,
			TracerProvider: telset.TracerProvider,
			MeterProvider:  telset.MeterProvider,
		},
		handler,
		xconfighttp.WithOtelHTTPOptions(
			otelhttp.WithFilter(func(r *http.Request) bool {
				ignorePath := path.Join("/", queryOpts.BasePath, "static")
				return !strings.HasPrefix(r.URL.Path, ignorePath)
			}),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				// Use just the route pattern without the HTTP method prefix or basePath
				// r.Pattern includes the method like "GET /jaeger/api/v3/traces/{trace_id}"
				// We want to return just "/api/v3/traces/{trace_id}" (without basePath)
				pattern := r.Pattern
				if pattern != "" {
					// Remove the method prefix (e.g., "GET ", "POST ", etc.)
					if idx := strings.Index(pattern, " "); idx > 0 {
						pattern = pattern[idx+1:]
					}
					// Remove basePath prefix if present
					if queryOpts.BasePath != "" && queryOpts.BasePath != "/" {
						pattern = strings.TrimPrefix(pattern, queryOpts.BasePath)
					}
				}
				return pattern
			}),
		),
	)
	if err != nil {
		return nil, errors.Join(err, staticHandlerCloser.Close())
	}
	server := &httpServer{
		Server:              hs,
		staticHandlerCloser: staticHandlerCloser,
	}

	return server, nil
}

func (hS httpServer) Close() error {
	var errs []error
	errs = append(errs,
		hS.Server.Close(),
		hS.staticHandlerCloser.Close(),
	)
	return errors.Join(errs...)
}

// initListener initialises listeners of the server
func (s *Server) initListener(ctx context.Context) error {
	var err error
	s.grpcConn, err = s.queryOptions.GRPC.NetAddr.Listen(ctx)
	if err != nil {
		return err
	}

	s.httpConn, err = s.queryOptions.HTTP.ToListener(ctx)
	if err != nil {
		return err
	}
	s.telset.Logger.Info(
		"Query server started",
		zap.String("http_addr", s.HTTPAddr()),
		zap.String("grpc_addr", s.GRPCAddr()),
	)
	return nil
}

// Start http and gRPC servers concurrently
func (s *Server) Start(ctx context.Context) error {
	err := s.initListener(ctx)
	if err != nil {
		return fmt.Errorf("query server failed to initialize listener: %w", err)
	}

	var httpPort int
	if port, err := getPortForAddr(s.httpConn.Addr()); err == nil {
		httpPort = port
	}

	var grpcPort int
	if port, err := getPortForAddr(s.grpcConn.Addr()); err == nil {
		grpcPort = port
	}

	s.bgFinished.Go(func() {
		s.telset.Logger.Info("Starting HTTP server", zap.Int("port", httpPort), zap.String("addr", s.queryOptions.HTTP.NetAddr.Endpoint))
		err := s.httpServer.Serve(s.httpConn)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.telset.Logger.Error("Could not start HTTP server", zap.Error(err))
			s.telset.ReportStatus(componentstatus.NewFatalErrorEvent(err))
			return
		}
		s.telset.Logger.Info("HTTP server stopped", zap.Int("port", httpPort), zap.String("addr", s.queryOptions.HTTP.NetAddr.Endpoint))
	})

	// Start GRPC server concurrently
	s.bgFinished.Go(func() {
		s.telset.Logger.Info("Starting GRPC server", zap.Int("port", grpcPort), zap.String("addr", s.queryOptions.GRPC.NetAddr.Endpoint))

		err := s.grpcServer.Serve(s.grpcConn)
		if err != nil {
			s.telset.Logger.Error("Could not start GRPC server", zap.Error(err))
			s.telset.ReportStatus(componentstatus.NewFatalErrorEvent(err))
			return
		}
		s.telset.Logger.Info("GRPC server stopped", zap.Int("port", grpcPort), zap.String("addr", s.queryOptions.GRPC.NetAddr.Endpoint))
	})
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
	var errs []error

	s.telset.Logger.Info("Closing HTTP server")
	if err := s.httpServer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close HTTP server: %w", err))
	}

	s.telset.Logger.Info("Stopping gRPC server")
	s.grpcServer.Stop()

	s.bgFinished.Wait()

	s.telset.Logger.Info("Server stopped")
	return errors.Join(errs...)
}
