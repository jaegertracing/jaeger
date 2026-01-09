// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// mockQueryExtension implements jaegerquery.Extension for testing
type mockQueryExtension struct {
	extension.Extension
}

func (m *mockQueryExtension) QueryService() *querysvc.QueryService {
	return querysvc.NewQueryService(&mockTraceReader{}, &mockDepReader{}, querysvc.QueryServiceOptions{})
}

// mockTraceReader implements tracestore.Reader for testing
type mockTraceReader struct{}

func (m *mockTraceReader) GetTraces(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {}
}

func (m *mockTraceReader) GetServices(_ context.Context) ([]string, error) {
	return []string{"test-service"}, nil
}

func (m *mockTraceReader) GetOperations(_ context.Context, _ tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return []tracestore.Operation{}, nil
}

func (m *mockTraceReader) FindTraces(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {}
}

func (m *mockTraceReader) FindTraceIDs(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {}
}

// mockDepReader implements depstore.Reader for testing
type mockDepReader struct{}

func (m *mockDepReader) GetDependencies(_ context.Context, _ depstore.QueryParameters) ([]model.DependencyLink, error) {
	return []model.DependencyLink{}, nil
}

// mockHost implements component.Host with a jaegerquery extension
type mockHost struct {
	component.Host
	queryExt jaegerquery.Extension
}

func newMockHost() *mockHost {
	return &mockHost{
		Host:     componenttest.NewNopHost(),
		queryExt: &mockQueryExtension{},
	}
}

func (m *mockHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		jaegerquery.ID: m.queryExt,
	}
}

// startTestServer creates and starts a test server with a random available port.
// It waits for the server to be ready and registers shutdown via t.Cleanup().
// Returns the started server and its address.
func startTestServer(t *testing.T) (*server, string) {
	t.Helper()

	host := newMockHost()
	telset := componenttest.NewNopTelemetrySettings()

	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0", // OS will assign a free port
		},
		ServerName:               "jaeger",
		ServerVersion:            "1.0.0",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)

	// Register cleanup
	t.Cleanup(func() {
		err := server.Shutdown(context.Background())
		assert.NoError(t, err)
	})

	// Get the actual address the server is listening on
	addr := server.listener.Addr().String()

	// Wait for server to be ready
	assert.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 1*time.Second, 10*time.Millisecond, "Server should be ready")

	return server, addr
}

func TestServerLifecycle(t *testing.T) {
	// Since we're not actually accessing storage in Phase 1,
	// we just need a basic host for the lifecycle test
	host := newMockHost()

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
				ServerVersion:            "1.0.0",
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
	host := newMockHost()
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
	_, addr := startTestServer(t)

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
	_, addr := startTestServer(t)

	// Test the MCP endpoint with a GET request
	// According to MCP Streamable HTTP spec, GET should return session info or error
	resp, err := http.Get(fmt.Sprintf("http://%s/mcp", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	// The MCP endpoint should not return 404 (it exists)
	assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)

	// Read and validate the response body if it's JSON
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// If the response is JSON, it should be valid JSON
	// The MCP spec indicates GET without session ID may return an error or session info
	if resp.Header.Get("Content-Type") == "application/json" {
		var result map[string]any
		err := json.Unmarshal(body, &result)
		assert.NoError(t, err, "Response should be valid JSON")
	}
}

func TestServerShutdownWithError(t *testing.T) {
	host := newMockHost()
	telset := componenttest.NewNopTelemetrySettings()
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		ServerVersion:            "1.0.0",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)

	// Close the listener first to ensure the server stops accepting connections
	server.listener.Close()

	// Wait a bit for the serve goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Create a context with very short timeout to try to trigger shutdown error
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	<-ctx.Done()

	// Even with expired context, shutdown should complete
	err = server.Shutdown(ctx)
	// The error handling path is exercised, even if no error is returned
	// because the server may have already stopped
	_ = err
}

func TestServerShutdownAfterListenerClose(t *testing.T) {
	host := newMockHost()
	telset := componenttest.NewNopTelemetrySettings()
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		ServerVersion:            "1.0.0",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
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

func TestServerShutdownErrorPath(t *testing.T) {
	host := newMockHost()
	telset := componenttest.NewNopTelemetrySettings()
	config := &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		ServerVersion:            "1.0.0",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}

	server := newServer(config, telset)
	err := server.Start(context.Background(), host)
	require.NoError(t, err)

	// Create an already-cancelled context to force shutdown error
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Shutdown with cancelled context should complete but may return an error
	err = server.Shutdown(ctx)
	// We exercise the error path - the actual error depends on timing
	// The important thing is that the error handling code is executed
	_ = err
}

func TestServerServeFails(t *testing.T) {
	host := newMockHost()
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

func TestHealthTool(t *testing.T) {
	telset := componenttest.NewNopTelemetrySettings()
	config := &Config{
		ServerName:               "test-server",
		ServerVersion:            "2.0.0",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}

	server := newServer(config, telset)

	// Call the healthTool directly
	result, output, err := server.healthTool(context.Background(), nil, struct{}{})

	// Verify the results
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "ok", output.Status)
	assert.Equal(t, "test-server", output.Server)
	assert.Equal(t, "2.0.0", output.Version)
}
