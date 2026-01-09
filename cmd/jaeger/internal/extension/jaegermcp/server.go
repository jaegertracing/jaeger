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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/tracesvc"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
)

var (
	_ extension.Extension             = (*server)(nil)
	_ extensioncapabilities.Dependent = (*server)(nil)
)

// server implements the Jaeger MCP extension.
type server struct {
	config      *Config
	telset      component.TelemetrySettings
	httpServer  *http.Server
	listener    net.Listener
	mcpServer   *mcp.Server
	traceSvc    *tracesvc.TraceService
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

	// Get storage readers from jaegerquery extension
	queryExt, err := jaegerquery.GetExtension(host)
	if err != nil {
		return fmt.Errorf("cannot get jaegerquery extension: %w", err)
	}

	// Create internal trace service layer for MCP tools
	s.traceSvc = tracesvc.NewTraceService(
		queryExt.V1SpanReader(),
		queryExt.V2TraceReader(),
		queryExt.DependencyReader(),
	)

	s.telset.Logger.Info("Successfully created trace service from jaegerquery extension")

	// Initialize MCP server with implementation details
	impl := &mcp.Implementation{
		Name:    s.config.ServerName,
		Version: s.config.ServerVersion,
	}
	// Pass empty ServerOptions to use default settings.
	// Custom options (e.g., logging, handlers) can be added in Phase 2 if needed.
	s.mcpServer = mcp.NewServer(impl, &mcp.ServerOptions{})

	// Register a placeholder health tool for Phase 1 Part 2
	// Actual MCP tools will be implemented in Phase 2
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "health",
		Description: "Check if the Jaeger MCP server is running",
	}, s.healthTool)

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
		Handler:           mux,
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
