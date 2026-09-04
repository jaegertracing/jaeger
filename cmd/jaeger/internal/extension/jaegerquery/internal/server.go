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
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp/xconfighttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/apiv3"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
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

// NewServer creates and initializes Server.
func NewServer(
	ctx context.Context,
	querySvc *querysvc.QueryService,
	metricsQuerySvc metricstore.Reader,
	options *QueryOptions,
	backendCaps BackendCapabilityProvider,
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
	httpServer, err := createHTTPServer(ctx, querySvc, metricsQuerySvc, options, backendCaps, tm, telset)
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

	if tm.Enabled {
		unaryInterceptors = append(unaryInterceptors, tenancy.NewGuardingUnaryInterceptor(tm))
		streamInterceptors = append(streamInterceptors, tenancy.NewGuardingStreamInterceptor(tm)) //nolint:contextcheck // The context is handled by the interceptors.
	}

	//nolint:contextcheck // The context is handled by the interceptors
	if len(options.HeaderForwarding) > 0 {
		unaryInterceptors = append(unaryInterceptors, headerforwarding.NewUnaryServerInterceptor(options.HeaderForwarding))
		streamInterceptors = append(streamInterceptors, headerforwarding.NewStreamServerInterceptor(options.HeaderForwarding))
	}

	grpcOpts = append(
		grpcOpts,
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
		grpcOpts...,
	)
}

// closers are the components initRouter mounts that own resources past a single
// request — the static assets handler, the AI gateway's MCP sessions, and whatever
// is mounted next. Being an io.Closer itself lets both the caller that owns the
// slice and the httpServer that stores it close the whole set the same way.
type closers []io.Closer

var _ io.Closer = closers(nil)

// Close closes every closer and joins the errors, so one failure does not hide the
// rest. This mirrors how the storage extension shuts its factories down.
func (cs closers) Close() error {
	var errs []error
	for _, closer := range cs {
		errs = append(errs, closer.Close())
	}
	return errors.Join(errs...)
}

type httpServer struct {
	*http.Server
	// closers shut down with the query server instead of being left to process exit.
	closers closers
}

var _ io.Closer = (*httpServer)(nil)

// initRouter returns, alongside the handler, the closers for everything it mounted
// that outlives a request; the caller owns closing them (see httpServer.closers).
func initRouter(
	ctx context.Context,
	querySvc *querysvc.QueryService,
	metricsQuerySvc metricstore.Reader,
	queryOpts *QueryOptions,
	backendCaps BackendCapabilityProvider,
	tenancyMgr *tenancy.Manager,
	telset telemetry.Settings,
) (http.Handler, closers, error) {
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.Logger(telset.Logger),
		HandlerOptions.Tracer(telset.TracerProvider),
		HandlerOptions.MetricsQueryService(metricsQuerySvc),
		HandlerOptions.BasePath(queryOpts.BasePath),
	}

	apiHandler := NewAPIHandler(
		querySvc,
		apiHandlerOptions...,
	)
	r := http.NewServeMux()
	var cs closers

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

	if queryOpts.AI.HasValue() {
		aiClosers, err := registerAIRoutes(ctx, r, queryOpts, querySvc, tenancyMgr, telset)
		if err != nil {
			return nil, nil, errors.Join(err, cs.Close())
		}
		cs = append(cs, aiClosers...)
	}

	if queryOpts.OTLPProxy.HasValue() {
		if err := registerOTLPProxy(r, queryOpts, telset); err != nil {
			return nil, nil, errors.Join(err, cs.Close())
		}
	}

	r.HandleFunc(apiNotFoundPattern, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "404 page not found", http.StatusNotFound)
	})

	cs = append(cs, RegisterStaticHandler(r, telset.Logger, queryOpts, backendCaps))

	// MUST wrap the mux directly: nothing may be inserted between the two, or the pattern
	// the mux records becomes invisible again. The wrappers below go on top of this one.
	handler := routeTagHandler(queryOpts.BasePath, r)
	if queryOpts.BearerTokenPropagation {
		handler = bearertoken.PropagationHandler(telset.Logger, handler)
	}
	if len(queryOpts.HeaderForwarding) > 0 {
		handler = headerforwarding.HTTPServerMiddleware(queryOpts.HeaderForwarding, handler)
	}
	if tenancyMgr.Enabled {
		handler = tenancy.ExtractTenantHTTPHandler(tenancyMgr, handler)
	}
	handler = traceResponseHandler(handler)
	return handler, cs, nil
}

// registerAIRoutes mounts the AI chat gateway and the telemetry MCP endpoint,
// and returns the closers for whatever it mounted. Invalid AI config disables
// the AI surface rather than stopping the query server, which also serves the
// UI and the trace APIs.
func registerAIRoutes(
	ctx context.Context,
	r *http.ServeMux,
	queryOpts *QueryOptions,
	querySvc *querysvc.QueryService,
	tenancyMgr *tenancy.Manager,
	telset telemetry.Settings,
) (closers, error) {
	aiCfg := queryOpts.AI.Get()
	if err := aiCfg.Validate(); err != nil {
		telset.Logger.Error("Invalid AI config, AI handler disabled", zap.Error(err))
		return nil, nil
	}

	var cs closers
	// One config for both MCP endpoints so they cannot drift, and the zero value
	// when MCP is off — the gateway ignores it then.
	var mcpCfg mcptools.Config
	if mcp := aiCfg.MCP.Get(); mcp != nil {
		mcpCfg = mcptools.DefaultConfig()
		// Operator overrides on top of the defaults. Zero means "unset" for all
		// three, which is why they are applied conditionally rather than copied.
		if mcp.MaxSpanDetailsPerRequest > 0 {
			mcpCfg.MaxSpanDetailsPerRequest = mcp.MaxSpanDetailsPerRequest
		}
		if mcp.MaxSearchResults > 0 {
			mcpCfg.MaxSearchResults = mcp.MaxSearchResults
		}
		if mcp.MaxReadFileSize > 0 {
			mcpCfg.MaxReadFileSize = mcp.MaxReadFileSize
		}
		// Opened once, so both endpoints share the handle and a broken path is
		// reported once, at startup. It stays open for as long as it serves, so
		// it is released with the server rather than at process exit.
		customSkills, err := mcptools.OpenCustomSkillsDir(mcp.SkillsDir)
		if err != nil {
			return nil, err
		}
		if customSkills != nil {
			cs = append(cs, customSkills)
		}
		mcpCfg.CustomSkillsFS = customSkills
		// Shared telemetry endpoint (/api/ai/mcp/). Coexists with the wildcard
		// turn-scoped pattern the gateway registers below.
		registerMCPTools(r, querySvc, tenancyMgr, queryOpts.BasePath, mcpCfg, telset)
	}

	if aiCfg.AgentURL != "" {
		// jaegerai owns the chat endpoint and, when MCP is on, the turn-scoped
		// endpoint (/api/ai/mcp/<id>/) — which is why mcpCfg is built above
		// rather than inside this branch. It holds MCP sessions past the request
		// that opened them, so it joins the closers.
		//
		// The announced base URL is resolved here because inferring the
		// gateway's own localhost address needs the query HTTP endpoint and TLS
		// setting, which live on QueryOptions, not AIConfig.
		aiGateway := jaegerai.NewHandler(jaegerai.HandlerParams{
			Logger:             telset.Logger,
			AgentURL:           aiCfg.AgentURL,
			AgentHeaders:       aiCfg.AgentHeaders,
			BasePath:           queryOpts.BasePath,
			MaxRequestBodySize: aiCfg.MaxRequestBodySize,
			EnableMCP:          aiCfg.MCP.HasValue(),
			MCPBaseURL:         aiCfg.resolveMCPBaseURL(ctx, queryOpts.HTTP.NetAddr.Endpoint, queryOpts.HTTP.TLS.HasValue()),
			QueryService:       querySvc,
			TenancyMgr:         tenancyMgr,
			Telset:             telset,
			MCPConfig:          mcpCfg,
		})
		aiGateway.RegisterRoutes(r)
		cs = append(cs, aiGateway)
	}
	return cs, nil
}

func otlpProxyPathPrefix(basePath string) string {
	prefix := "/api/otlp"
	if basePath != "" && basePath != "/" {
		prefix = basePath + prefix
	}
	return prefix
}

func otelFilterFunc(basePath string) func(*http.Request) bool {
	prefixes := []string{
		path.Join("/", basePath, "static"),
		otlpProxyPathPrefix(basePath),
	}
	return func(r *http.Request) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				return false
			}
		}
		return true
	}
}

func registerMCPTools(r *http.ServeMux, querySvc *querysvc.QueryService, tenancyMgr *tenancy.Manager, basePath string, cfg mcptools.Config, telset telemetry.Settings) {
	handler := mcptools.NewHandler(telset, querySvc, tenancyMgr, cfg)
	prefix := strings.TrimSuffix(basePath, "/") + "/api/ai/mcp"
	r.Handle(prefix+"/", http.StripPrefix(prefix, handler))
	telset.Logger.Info("Jaeger telemetry MCP endpoint enabled", zap.String("path", prefix+"/"))
}

// per-route wrap is the only instrumentation layer.
func registerOTLPProxy(r *http.ServeMux, queryOpts *QueryOptions, telset telemetry.Settings) error {
	cfg := queryOpts.OTLPProxy.Get()
	target, err := url.Parse(cfg.Target)
	if err != nil {
		return fmt.Errorf("invalid OTLP proxy target %q: %w", cfg.Target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	prefix := otlpProxyPathPrefix(queryOpts.BasePath)
	instrumented := otelhttp.NewHandler(
		http.StripPrefix(prefix, proxy),
		"otlp.proxy",
		otelhttp.WithTracerProvider(nooptrace.NewTracerProvider()),
		otelhttp.WithMeterProvider(telset.MeterProvider),
	)
	r.Handle(prefix+"/v1/", instrumented)
	telset.Logger.Info("OTLP proxy registered",
		zap.String("path_prefix", prefix+"/v1/"),
		zap.String("target", cfg.Target))
	return nil
}

func createHTTPServer(
	ctx context.Context,
	querySvc *querysvc.QueryService,
	metricsQuerySvc metricstore.Reader,
	queryOpts *QueryOptions,
	backendCaps BackendCapabilityProvider,
	tm *tenancy.Manager,
	telset telemetry.Settings,
) (*httpServer, error) {
	handler, cs, err := initRouter(ctx, querySvc, metricsQuerySvc, queryOpts, backendCaps, tm, telset)
	if err != nil {
		return nil, err
	}
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
			otelhttp.WithFilter(otelFilterFunc(queryOpts.BasePath)),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return spanNameForRoute(routeFromPattern(r.Pattern), queryOpts.BasePath)
			}),
		),
	)
	if err != nil {
		return nil, errors.Join(err, cs.Close())
	}
	server := &httpServer{
		Server:  hs,
		closers: cs,
	}

	return server, nil
}

func (hS httpServer) Close() error {
	return errors.Join(hS.Server.Close(), hS.closers.Close())
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
