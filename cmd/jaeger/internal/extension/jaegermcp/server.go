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

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
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
func (s *server) Start(_ context.Context, host component.Host) error {
	s.telset.Logger.Info("Starting Jaeger MCP server", zap.String("endpoint", s.config.HTTP.Endpoint))

	// TODO Phase 2: Get QueryService from jaegerquery extension
	// This will require jaegerquery to expose QueryService through an Extension interface,
	// similar to how jaegerstorage exposes storage factories.
	// For now, we just verify that jaegerquery extension is available.
	_ = host

	// TODO: Initialize MCP server with Streamable HTTP transport
	// This will be implemented in Phase 2 once we add the MCP SDK dependency

	// For Phase 1, we just set up a basic HTTP server to validate the extension lifecycle
	//nolint:noctx // Phase 1 temporary implementation, will be replaced with MCP SDK in Phase 2
	listener, err := net.Listen("tcp", s.config.HTTP.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.HTTP.Endpoint, err)
	}
	s.listener = listener

	// Create a basic HTTP server
	mux := http.NewServeMux()
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

	s.telset.Logger.Info("Jaeger MCP server started successfully", zap.String("endpoint", s.config.HTTP.Endpoint))
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
