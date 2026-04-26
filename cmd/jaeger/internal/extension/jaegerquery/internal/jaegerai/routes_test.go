// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewHandlerInitialisesStore(t *testing.T) {
	h := NewHandler(zap.NewNop(), "ws://example", "/jaeger", 1<<20)
	require.NotNil(t, h.store, "NewHandler must allocate a ContextualToolsStore")
	assert.Equal(t, "ws://example", h.agentURL)
	assert.Equal(t, "/jaeger", h.basePath)
	assert.Equal(t, int64(1<<20), h.maxRequestBodySize)
}

func TestRegisterRoutesMountsBothEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		wantChat    string
		wantMCPHead string
	}{
		{
			name:        "no base path",
			basePath:    "",
			wantChat:    "/api/ai/chat",
			wantMCPHead: "/api/ai/mcp/",
		},
		{
			name:        "single-slash base path is treated as no prefix",
			basePath:    "/",
			wantChat:    "/api/ai/chat",
			wantMCPHead: "/api/ai/mcp/",
		},
		{
			name:        "with base path",
			basePath:    "/jaeger",
			wantChat:    "/jaeger/api/ai/chat",
			wantMCPHead: "/jaeger/api/ai/mcp/",
		},
		{
			// Operator-supplied trailing slash must be normalized away so we
			// don't register a "/jaeger//api/..." pattern that diverges from
			// the URL the contextual MCP handler builds for the sidecar.
			name:        "trailing slash in base path is normalized",
			basePath:    "/jaeger/",
			wantChat:    "/jaeger/api/ai/chat",
			wantMCPHead: "/jaeger/api/ai/mcp/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(zap.NewNop(), "ws://127.0.0.1:1", tc.basePath, 1<<20)
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)

			// Chat endpoint: GET (wrong method) is enough to confirm the
			// route is mounted — the handler returns 405 instead of the mux
			// returning 404.
			req := httptest.NewRequest(http.MethodGet, tc.wantChat, http.NoBody)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code,
				"chat endpoint should be mounted at %s", tc.wantChat)

			// MCP endpoint: hit the wildcard with any id and confirm the
			// MCP handler responds (any non-404 means the route matched).
			req = httptest.NewRequest(http.MethodGet, tc.wantMCPHead+"some-id", http.NoBody)
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.NotEqual(t, http.StatusNotFound, rr.Code,
				"MCP endpoint should be mounted under %s{contextual_mcp_id}", tc.wantMCPHead)
		})
	}
}

func TestBasePathNormalizationAgreesAcrossRoutesAndURL(t *testing.T) {
	// Regression: registering with "/jaeger/" must not produce a mux
	// pattern with "//", and buildContextualMCPURL must not strip the slash
	// independently. Both must converge on a single canonical "/jaeger".
	const dirty = "/jaeger/"
	h := NewHandler(zap.NewNop(), "ws://127.0.0.1:1", dirty, 1<<20)

	assert.Equal(t, "/jaeger", h.basePath, "NewHandler should normalize the trailing slash")

	chatHandler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", dirty, 1<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", http.NoBody)
	req.Host = "gateway.example"
	url := chatHandler.buildContextualMCPURL(req, "id-x")

	assert.Equal(t, "http://gateway.example/jaeger/api/ai/mcp/id-x", url,
		"URL builder must use the same normalized prefix as RegisterRoutes")
	assert.NotContains(t, url, "//api", "no double slash before /api/...")
}

func TestRegisterRoutesChatHandlerSharesStore(t *testing.T) {
	// Confirm the chat handler and the MCP handler use the same store
	// instance, so a tools snapshot SetForContextualMCPID-ed by the chat
	// path is visible to the MCP path.
	h := NewHandler(zap.NewNop(), "ws://127.0.0.1:1", "", 1<<20)
	h.store.SetForContextualMCPID("turn-1", nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/mcp/turn-1", strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Any non-404 response confirms the MCP handler was reached with the
	// shared store — exact MCP semantics are covered in contextual_mcp_test.go.
	assert.NotEqual(t, http.StatusNotFound, rr.Code)
}
