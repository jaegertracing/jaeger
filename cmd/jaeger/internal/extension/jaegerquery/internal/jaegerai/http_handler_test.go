// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewHandlerBuildsEndpoints(t *testing.T) {
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://example", BasePath: "/jaeger", MaxRequestBodySize: 1 << 20, Telset: telemetry.NoopSettings(), TenancyMgr: tenancy.NewManager(&tenancy.Options{})})
	require.NotNil(t, h.chat, "NewHandler must build the chat endpoint")
	assert.Equal(t, "/jaeger", h.basePath)
	assert.NotNil(t, h.mcp, "the turn-scoped endpoint is built for a chat gateway even when EnableMCP is false (it carries the turn's UI tools)")
}

func TestRegisterRoutesMountsChatEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		wantChat string
	}{
		{
			name:     "no base path",
			basePath: "",
			wantChat: "/api/ai/chat",
		},
		{
			name:     "single-slash base path is treated as no prefix",
			basePath: "/",
			wantChat: "/api/ai/chat",
		},
		{
			name:     "with base path",
			basePath: "/jaeger",
			wantChat: "/jaeger/api/ai/chat",
		},
		{
			// Operator-supplied trailing slash must be normalized away so we
			// don't register a "/jaeger//api/..." pattern.
			name:     "trailing slash in base path is normalized",
			basePath: "/jaeger/",
			wantChat: "/jaeger/api/ai/chat",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: tc.basePath, MaxRequestBodySize: 1 << 20, Telset: telemetry.NoopSettings(), TenancyMgr: tenancy.NewManager(&tenancy.Options{})})
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)

			// Chat endpoint: GET (wrong method) is enough to confirm the
			// route is mounted — the handler returns 405 instead of the
			// mux returning 404.
			req := httptest.NewRequest(http.MethodGet, tc.wantChat, http.NoBody)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code,
				"chat endpoint should be mounted at %s", tc.wantChat)
		})
	}
}

func TestNewHandlerNormalizesTrailingSlash(t *testing.T) {
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: "/jaeger/", MaxRequestBodySize: 1 << 20, Telset: telemetry.NoopSettings(), TenancyMgr: tenancy.NewManager(&tenancy.Options{})})
	assert.Equal(t, "/jaeger", h.basePath, "NewHandler must trim the trailing slash")
}

// TestNewHandlerNormalizesMCPBaseURL pins the trailing-slash handling: config only
// requires an absolute URL, so an operator may legally write trailing slashes, and
// an announced "…//api/ai/mcp/<id>/" is a path the mux never matches. NewHandler
// must trim them so the announced URL has exactly one slash before the route.
func TestNewHandlerNormalizesMCPBaseURL(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	for _, base := range []string{
		"http://127.0.0.1:16686",
		"http://127.0.0.1:16686/",
		"http://127.0.0.1:16686//",
	} {
		h := NewHandler(HandlerParams{
			Logger: zap.NewNop(), AgentURL: "ws://x", MaxRequestBodySize: 1,
			EnableMCP: true, QueryService: svc, TenancyMgr: tenancy.NewManager(&tenancy.Options{}),
			Telset: telemetry.NoopSettings(), MCPBaseURL: base,
		})
		got := h.chat.announceMCP(httpCaps(true), "SID")
		require.Len(t, got, 1)
		assert.Equal(t, "http://127.0.0.1:16686/api/ai/mcp/SID/", got[0].Http.Url,
			"base URL %q must normalize to a single slash", base)
	}
}

func mcpEnabledHandler(t *testing.T, basePath string) *Handler {
	t.Helper()
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	return NewHandler(HandlerParams{
		Logger:             zap.NewNop(),
		AgentURL:           "ws://127.0.0.1:1",
		BasePath:           basePath,
		MaxRequestBodySize: 1 << 20,
		EnableMCP:          true,
		QueryService:       svc,
		TenancyMgr:         tenancy.NewManager(&tenancy.Options{}),
		Telset:             telemetry.NoopSettings(),
	})
}

func TestRegisterRoutesMountsSessionScopedMCPWhenEnabled(t *testing.T) {
	h := mcpEnabledHandler(t, "")
	require.NotNil(t, h.mcp, "MCP endpoint must be built when EnableMCP is true")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	routeID := registerTurn(h.mcp.turns, testStreamingClient(), nil) // active turn

	t.Run("active turn is served", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/"+routeID+"/mcp", http.NoBody))
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "registered turn must reach the MCP handler")
	})

	t.Run("active turn is served without a trailing slash", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/"+routeID, http.NoBody))
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "no-slash form must also reach the MCP handler")
	})

	t.Run("unknown route id is rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/ghost/mcp", http.NoBody))
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

// TestRegisterRoutesMountsTurnScopedMCPWithoutTelemetry is the M7.2 behavior at the
// route level: even with the telemetry MCP server disabled (no ai.mcp block), a
// chat gateway still mounts the turn-scoped endpoint so the turn's UI tools can
// dispatch over MCP. A registered turn is served; an unknown one is still 404.
func TestRegisterRoutesMountsTurnScopedMCPWithoutTelemetry(t *testing.T) {
	h := NewHandler(HandlerParams{
		Logger:             zap.NewNop(),
		AgentURL:           "ws://127.0.0.1:1",
		BasePath:           "",
		MaxRequestBodySize: 1 << 20,
		EnableMCP:          false,
		Telset:             telemetry.NoopSettings(),
		TenancyMgr:         tenancy.NewManager(&tenancy.Options{}),
	})
	require.NotNil(t, h.mcp, "turn-scoped endpoint must be mounted for a chat gateway even without an ai.mcp block")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	routeID := registerTurn(h.mcp.turns, testStreamingClient(), nil)

	t.Run("active turn is served", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/"+routeID+"/mcp", http.NoBody))
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "a registered turn must reach the endpoint even without telemetry")
	})

	t.Run("unknown route id is rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/ghost/mcp", http.NoBody))
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

// TestHandlerCloseReapsMCPSessions pins the gateway into jaeger-query's teardown
// chain (Server.Close → httpServer.Close → closers.Close → here). The MCP SDK reaps a
// session only when it goes idle, so without this a live session would outlive the
// server that served it. It binds a real client session over the turn-scoped endpoint,
// then asserts Close tears it down — a call afterward fails. (The reaping mechanism
// itself is unit-tested in mcptools.TestHandlerCloseReapsSessions; this pins the
// gateway's Close through to it.)
func TestHandlerCloseReapsMCPSessions(t *testing.T) {
	h := mcpEnabledHandler(t, "")
	require.Implements(t, (*io.Closer)(nil), h, "the gateway must be closable by the server's teardown chain")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	routeID := registerTurn(h.mcp.turns, testStreamingClient(), nil)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+routeID+"/")
	_, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err, "the session works before Close")

	require.NoError(t, h.Close())

	_, err = session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.Error(t, err, "after Close the session is reaped, so a further call fails")
}

// TestHandlerCloseIsNoOpWhenNothingToClose covers the shapes jaeger-query holds
// when the gateway is not fully enabled: a nil Handler (no AI at all), and a
// chat-only Handler whose turn-scoped endpoint has no bound sessions. Both must
// close to nothing, because httpServer.Close calls straight through without a guard.
func TestHandlerCloseIsNoOpWhenNothingToClose(t *testing.T) {
	var nilHandler *Handler
	require.NoError(t, nilHandler.Close(), "a nil gateway must be closable")

	chatOnly := NewHandler(HandlerParams{
		Logger:             zap.NewNop(),
		AgentURL:           "ws://127.0.0.1:1",
		MaxRequestBodySize: 1 << 20,
		Telset:             telemetry.NoopSettings(),
		TenancyMgr:         tenancy.NewManager(&tenancy.Options{}),
	})
	require.NotNil(t, chatOnly.mcp, "chat mounts the turn-scoped endpoint for UI tools")
	require.NoError(t, chatOnly.Close(), "closing with no bound sessions is a no-op")
}
