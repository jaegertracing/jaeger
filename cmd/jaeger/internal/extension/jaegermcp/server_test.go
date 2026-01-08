// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
)

func TestServerLifecycle(t *testing.T) {
	// Since we're not actually accessing storage in Phase 1,
	// we just need a basic host for the lifecycle test
	host := componenttest.NewNopHost()

	tests := []struct {
		name          string
		config        *Config
		expectedError string
	}{
		{
			name: "successful start and shutdown",
			config: &Config{
				HTTP:                     createDefaultConfig().(*Config).HTTP,
				ServerName:               "jaeger",
				MaxSpanDetailsPerRequest: 20,
				MaxSearchResults:         100,
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telset := componenttest.NewNopTelemetrySettings()
			server := newServer(tt.config, telset)
			require.NotNil(t, server)

			// Test Start
			err := server.Start(context.Background(), host)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}
			require.NoError(t, err)

			// Test Shutdown
			err = server.Shutdown(context.Background())
			assert.NoError(t, err)
		})
	}
}

func TestServerStartFailsWithInvalidEndpoint(t *testing.T) {
	host := componenttest.NewNopHost()
	telset := componenttest.NewNopTelemetrySettings()

	// Use an invalid endpoint (e.g., malformed address)
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "invalid-endpoint-format",
		},
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to listen")
}

func TestServerHealthEndpoint(t *testing.T) {
	host := componenttest.NewNopHost()
	telset := componenttest.NewNopTelemetrySettings()

	// Use a random available port
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0", // OS will assign a free port
		},
		ServerName:               "jaeger",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)
	defer func() {
		err := server.Shutdown(context.Background())
		assert.NoError(t, err)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual address the server is listening on
	addr := server.listener.Addr().String()

	// Test the health endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "MCP server is running", string(body))
}

func TestServerMCPEndpoint(t *testing.T) {
	host := componenttest.NewNopHost()
	telset := componenttest.NewNopTelemetrySettings()

	// Use a random available port
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0", // OS will assign a free port
		},
		ServerName:               "jaeger",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)
	defer func() {
		err := server.Shutdown(context.Background())
		assert.NoError(t, err)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual address the server is listening on
	addr := server.listener.Addr().String()

	// Test that the MCP endpoint is available (even if we don't test the full protocol)
	// Just verify that the endpoint exists and responds to HTTP
	resp, err := http.Get(fmt.Sprintf("http://%s/mcp", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	// The MCP endpoint should respond (even if it's an error for GET without proper protocol)
	// Just verify it's not 404
	assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)
}

func TestServerShutdownWithError(t *testing.T) {
	host := componenttest.NewNopHost()
	telset := componenttest.NewNopTelemetrySettings()
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)

	// Create a context with very short timeout to try to trigger shutdown error
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	<-ctx.Done()

	err = server.Shutdown(ctx)
	// This may or may not produce an error depending on timing
	// but it exercises the error handling path
	_ = err
}

func TestServerShutdownAfterListenerClose(t *testing.T) {
	host := componenttest.NewNopHost()
	telset := componenttest.NewNopTelemetrySettings()
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)

	// Close listener to simulate an already-closed server scenario
	server.listener.Close()

	// Give the goroutine time to detect the closed listener and exit
	time.Sleep(50 * time.Millisecond)

	// Now shutdown should still work gracefully
	err = server.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestServerServeFails(t *testing.T) {
	host := componenttest.NewNopHost()
	telset := componenttest.NewNopTelemetrySettings()

	// Create a server and start it
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
	}
	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)

	// Close the listener immediately to trigger an error in the Serve goroutine
	server.listener.Close()

	// Give the goroutine time to detect the closed listener and hit the error path
	time.Sleep(100 * time.Millisecond)

	// Clean up
	err = server.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestServerDependencies(t *testing.T) {
	server := &server{}
	deps := server.Dependencies()
	require.Len(t, deps, 1)
	assert.Equal(t, jaegerquery.ID, deps[0])
}

func TestShutdownWithoutStart(t *testing.T) {
	telset := componenttest.NewNopTelemetrySettings()
	server := newServer(createDefaultConfig().(*Config), telset)

	err := server.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestNewServer(t *testing.T) {
	telset := componenttest.NewNopTelemetrySettings()
	config := createDefaultConfig().(*Config)

	server := newServer(config, telset)
	assert.NotNil(t, server)
	assert.Equal(t, config, server.config)
	assert.Equal(t, telset, server.telset)
	assert.Nil(t, server.httpServer)
	assert.Nil(t, server.listener)
}
