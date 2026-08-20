// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestNewHandlerBuildsEndpoints(t *testing.T) {
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://example", BasePath: "/jaeger", MaxRequestBodySize: 1 << 20})
	require.NotNil(t, h.chat, "NewHandler must build the chat endpoint")
	assert.Equal(t, "/jaeger", h.basePath)
	assert.Nil(t, h.mcp, "the MCP endpoint must be nil when EnableMCP is false")
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
			h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: tc.basePath, MaxRequestBodySize: 1 << 20})
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
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: "/jaeger/", MaxRequestBodySize: 1 << 20})
	assert.Equal(t, "/jaeger", h.basePath, "NewHandler must trim the trailing slash")
}

// TestNewHandlerNormalizesMCPBaseURL pins the trailing-slash handling: config only
// requires an absolute URL, so an operator may legally write trailing slashes, and
// an announced "…//api/ai/mcp/<id>/" is a path the mux never matches. NewHandler
// must trim them so the announced URL has exactly one slash before the route.
func TestNewHandlerNormalizesMCPBaseURL(t *testing.T) {
	for _, base := range []string{
		"http://127.0.0.1:16686",
		"http://127.0.0.1:16686/",
		"http://127.0.0.1:16686//",
	} {
		h := NewHandler(HandlerParams{
			Logger: zap.NewNop(), AgentURL: "ws://x", MaxRequestBodySize: 1,
			MCP:    sharedMCPHandler(t),
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
	return NewHandler(HandlerParams{
		Logger:             zap.NewNop(),
		AgentURL:           "ws://127.0.0.1:1",
		BasePath:           basePath,
		MaxRequestBodySize: 1 << 20,
		MCP:                sharedMCPHandler(t),
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

// TestRegisterRoutesOmitsMCPEndpointWhenDisabled is the chat-only shape: with no
// ai.mcp block the query server passes no MCP handler, so there are no telemetry
// tools to attach UI tools to and the turn-scoped endpoint is not built at all.
func TestRegisterRoutesOmitsMCPEndpointWhenDisabled(t *testing.T) {
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: "", MaxRequestBodySize: 1 << 20})
	require.Nil(t, h.mcp, "no MCP handler supplied means no turn-scoped endpoint")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// With MCP disabled the route is never mounted, so any turn URL is a 404
	// regardless of whether a turn is active.
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/any-id/mcp", http.NoBody))
	assert.Equal(t, http.StatusNotFound, rr.Code, "turn-scoped MCP endpoint must not be mounted when disabled")
}

// TestSharedCloseReapsTurnScopedSessions pins the turn-scoped endpoint into
// jaeger-query's teardown chain. The gateway holds nothing itself — its turn-scoped
// endpoint borrows the shared MCP handler — so the sessions it binds must be reaped
// when the query server closes that handler (Server.Close → httpServer.Close →
// closers.Close → mcptools.Handler.Close). The MCP SDK reaps a session only when it
// goes idle, so without this a live turn's session would outlive the server.
func TestSharedCloseReapsTurnScopedSessions(t *testing.T) {
	shared := sharedMCPHandler(t)
	h := NewHandler(HandlerParams{
		Logger:             zap.NewNop(),
		AgentURL:           "ws://127.0.0.1:1",
		MaxRequestBodySize: 1 << 20,
		MCP:                shared,
		Telset:             telemetry.NoopSettings(),
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	routeID := registerTurn(h.mcp.turns, testStreamingClient(), nil)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+routeID+"/")
	_, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err, "the session works before Close")

	require.NoError(t, shared.Close())

	_, err = session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.Error(t, err, "closing the shared handler must reap the turn-scoped session too")
}
