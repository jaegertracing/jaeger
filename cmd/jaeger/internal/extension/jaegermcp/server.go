// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/handlers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

var (
	_ extension.Extension             = (*server)(nil)
	_ extensioncapabilities.Dependent = (*server)(nil)
)

// server implements the Jaeger MCP extension.
type server struct {
	config     *Config
	telset     component.TelemetrySettings
	httpServer *http.Server
	listener   net.Listener
	mcpServer  *mcp.Server
	queryAPI   *querysvc.QueryService
}

// newServer creates a new MCP server instance.
func newServer(config *Config, telset component.TelemetrySettings) *server {
	return &server{
		config: config,
		telset: telset,
	}
}

// Dependencies implements extensioncapabilities.Dependent to ensure
// this extension starts after the jaegerquery extension.
func (*server) Dependencies() []component.ID {
	return []component.ID{jaegerquery.ID}
}

// Start initializes and starts the MCP server.
func (s *server) Start(ctx context.Context, host component.Host) error {
	s.telset.Logger.Info("Starting Jaeger MCP server", zap.String("endpoint", s.config.HTTP.Endpoint))

	// Get v2 QueryService from jaegerquery extension
	queryExt, err := jaegerquery.GetExtension(host)
	if err != nil {
		return fmt.Errorf("cannot get %s extension: %w", jaegerquery.ID, err)
	}
	s.queryAPI = queryExt.QueryService()
	s.telset.Logger.Info("Successfully retrieved v2 QueryService from jaegerquery extension")

	// Initialize MCP server with implementation details
	impl := &mcp.Implementation{
		Name:    s.config.ServerName,
		Version: s.config.ServerVersion,
	}
	// Pass empty ServerOptions to use default settings.
	// Custom options (e.g., logging, handlers) can be added in Phase 2 if needed.
	s.mcpServer = mcp.NewServer(impl, &mcp.ServerOptions{})

	// Register MCP tools
	s.registerTools()

	// Set up TCP listener with context
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", s.config.HTTP.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.HTTP.Endpoint, err)
	}
	s.listener = listener

	// Create MCP streamable HTTP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return s.mcpServer },
		&mcp.StreamableHTTPOptions{
			JSONResponse:   false, // Use SSE for streamed events
			Stateless:      false, // Session state management
			SessionTimeout: 5 * time.Minute,
		},
	)

	// Create HTTP server with MCP handler and health endpoint
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("MCP server is running"))
	})

	s.httpServer = &http.Server{
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 30 * time.Second,
	}

	// Start the server in a goroutine
	go func() {
		if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.telset.Logger.Error("MCP server error", zap.Error(err))
		}
	}()

	s.telset.Logger.Info("Jaeger MCP server started successfully",
		zap.String("endpoint", s.config.HTTP.Endpoint),
		zap.String("mcp_endpoint", "http://"+s.config.HTTP.Endpoint+"/mcp"))
	return nil
}

// Shutdown gracefully stops the MCP server.
func (s *server) Shutdown(ctx context.Context) error {
	s.telset.Logger.Info("Shutting down Jaeger MCP server")

	var errs []error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown HTTP server: %w", err))
		}
	}

	return errors.Join(errs...)
}

// registerTools registers all MCP tools with the server.
func (s *server) registerTools() {
	// Get services tool (at the top - required for search_traces)
	getServicesHandler := handlers.NewGetServicesHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_services",
		Description: "List available service names. Use this first to discover valid service names for search_traces.",
	}, getServicesHandler)

	// Get span names tool (required for search_traces with span name filter)
	getSpanNamesHandler := handlers.NewGetSpanNamesHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_span_names",
		Description: "List available span names for a service. Supports regex filtering and span kind filtering.",
	}, getSpanNamesHandler)

	// Health check tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "health",
		Description: "Check if the Jaeger MCP server is running",
	}, s.healthTool)

	// Search traces tool
	searchTracesHandler := handlers.NewSearchTracesHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_traces",
		Description: "Find traces matching service, time, attributes, and duration criteria. Returns trace summary only.",
	}, searchTracesHandler)

	// Get span details tool
	getSpanDetailsHandler := handlers.NewGetSpanDetailsHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_span_details",
		Description: "Fetch full details (attributes, events, links, status) for specific spans.",
	}, getSpanDetailsHandler)

	// Get trace errors tool
	getTraceErrorsHandler := handlers.NewGetTraceErrorsHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_trace_errors",
		Description: "Get full details for all spans with error status.",
	}, getTraceErrorsHandler)

	// Get trace topology tool
	getTraceTopologyHandler := handlers.NewGetTraceTopologyHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_trace_topology",
		Description: "Get the structural tree of a trace showing parent-child relationships, timing, and error locations. Does NOT return attributes or logs.",
	}, getTraceTopologyHandler)

	// Get critical path tool
	getCriticalPathHandler := handlers.NewGetCriticalPathHandler(s.queryAPI)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_critical_path",
		Description: "Identify the sequence of spans forming the critical latency path (the blocking execution path).",
	}, getCriticalPathHandler)
}

// HealthToolOutput is the strongly-typed output for the health tool.
type HealthToolOutput struct {
	Status  string `json:"status" jsonschema:"Server status (ok/error)"`
	Server  string `json:"server" jsonschema:"Server name"`
	Version string `json:"version" jsonschema:"Server version"`
}

// healthTool is a placeholder MCP tool that checks server health.
// Actual MCP tools for trace querying will be implemented in Phase 2.
func (s *server) healthTool(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, HealthToolOutput, error) {
	return nil, HealthToolOutput{
		Status:  "ok",
		Server:  s.config.ServerName,
		Version: s.config.ServerVersion,
	}, nil
}

// corsMiddleware wraps an http.Handler to add CORS headers.
// This is required for browser-based MCP clients like the MCP Inspector.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-ID")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
