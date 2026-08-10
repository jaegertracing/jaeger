// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// connectTestClient starts an httptest server around the handler and returns a
// connected MCP client session.
func connectTestClient(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })
	return session
}

// TestNewHandler_ListTools drives the full in-process serving stack (mcp.Server
// + middleware + tenancy + otelhttp) over HTTP and asserts the telemetry tools
// are advertised. ListTools does not touch storage, so the QueryService is
// backed by empty mocks.
func TestNewHandler_ListTools(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	handler := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())

	session := connectTestClient(t, handler)
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"get_services", "get_span_names", "search_traces", "get_span_details",
		"get_trace_errors", "get_trace_topology", "get_critical_path", "get_service_dependencies",
		"read_skill",
	}, got)
}

// TestNewHandler_CallTool exercises a tool end-to-end through the HTTP stack,
// confirming the handler reaches the QueryService and returns a result.
func TestNewHandler_CallTool(t *testing.T) {
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Return([]string{"svc-a", "svc-b"}, nil)
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	handler := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())

	session := connectTestClient(t, handler)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "svc-a")
	assert.Contains(t, text.Text, "svc-b")
}

// TestNewServerDegradesWithoutMetrics covers the branch where the metrics
// middleware fails to build: the server is still returned (metrics degraded)
// rather than the construction failing.
func TestNewServerDegradesWithoutMetrics(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	telset := telemetry.NoopSettings()
	telset.MeterProvider = &failingMeterProvider{failCounter: true}

	srv := NewServer(telset, svc, DefaultConfig())
	require.NotNil(t, srv)
}

// TestHandlerCloseReapsSessions pins the shared endpoint into the query server's
// teardown (M7.1): NewHandler's server holds a session until it goes idle, so Close
// must reap it or it outlives the server that served it.
func TestHandlerCloseReapsSessions(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())

	// Bind a session the way an MCP client on the HTTP transport does; nothing else
	// owns it, so nothing else would ever reap it.
	serverTransport, _ := mcp.NewInMemoryTransports()
	_, err := h.server.Connect(context.Background(), serverTransport, nil)
	require.NoError(t, err)
	require.NotEmpty(t, slices.Collect(h.server.Sessions()), "precondition: a session is bound")

	require.NoError(t, h.Close())
	assert.Empty(t, slices.Collect(h.server.Sessions()), "Close must reap the shared endpoint's sessions")
}

// TestHandlerAddReceivingMiddleware covers the hook the AI gateway uses to layer its
// per-turn UI tools onto a server it did not build. NewHandler captures the server by
// pointer, so middleware registered after the handler exists must still run for every
// later request — otherwise a gateway attaching to the shared endpoint would silently
// serve telemetry tools only.
func TestHandlerAddReceivingMiddleware(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	t.Cleanup(func() { require.NoError(t, h.Close()) })

	var saw atomic.Bool
	h.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				saw.Store(true)
			}
			return next(ctx, method, req)
		}
	})

	session := connectTestClient(t, h)
	_, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	assert.True(t, saw.Load(), "middleware added after the handler is built must run for later requests")
}

// TestRegisterTools verifies RegisterTools advertises the full tool set on a
// bare server (in-memory transport, no HTTP stack). Registration only, so the
// QueryService is backed by empty mocks that are never invoked.
func TestRegisterTools(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	registerTools(server, svc, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	listed, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}

	assert.ElementsMatch(t, []string{
		"get_services", "get_span_names", "search_traces", "get_span_details",
		"get_trace_errors", "get_trace_topology", "get_critical_path", "get_service_dependencies",
		"read_skill",
	}, got)
}
