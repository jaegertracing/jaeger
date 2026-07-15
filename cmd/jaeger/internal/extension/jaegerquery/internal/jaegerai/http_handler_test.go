// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewHandlerInitialisesStore(t *testing.T) {
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://example", BasePath: "/jaeger", MaxRequestBodySize: 1 << 20})
	require.NotNil(t, h.store, "NewHandler must allocate a ContextualToolsStore")
	require.NotNil(t, h.turns, "NewHandler must allocate a turnRegistry")
	assert.Equal(t, "ws://example", h.agentURL)
	assert.Equal(t, "/jaeger", h.basePath)
	assert.Equal(t, int64(1<<20), h.maxRequestBodySize)
	assert.Nil(t, h.mcpHandler, "MCP handler must be nil when EnableMCP is false")
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
	require.NotNil(t, h.mcpHandler, "MCP handler must be built when EnableMCP is true")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.turns.set("sess-1", testStreamingClient(), nil) // active turn

	t.Run("active session is served", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/sess-1/mcp", http.NoBody))
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "registered session must reach the MCP handler")
	})

	t.Run("active session is served without a trailing slash", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/sess-1", http.NoBody))
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "no-slash form must also reach the MCP handler")
	})

	t.Run("unknown session is rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/ghost/mcp", http.NoBody))
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestRegisterRoutesOmitsMCPEndpointWhenDisabled(t *testing.T) {
	h := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: "", MaxRequestBodySize: 1 << 20})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.turns.set("sess-1", testStreamingClient(), nil)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/sess-1/mcp", http.NoBody))
	assert.Equal(t, http.StatusNotFound, rr.Code, "turn-scoped MCP endpoint must not be mounted when disabled")
}
