// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestNewHandlerBuildsEndpoints(t *testing.T) {
	h, err := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://example", BasePath: "/jaeger", MaxRequestBodySize: 1 << 20})
	require.NoError(t, err)
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
			h, err := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: tc.basePath, MaxRequestBodySize: 1 << 20})
			require.NoError(t, err)
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
	h, err := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: "/jaeger/", MaxRequestBodySize: 1 << 20})
	require.NoError(t, err)
	assert.Equal(t, "/jaeger", h.basePath, "NewHandler must trim the trailing slash")
}

func mcpEnabledHandler(t *testing.T, basePath string) *Handler {
	t.Helper()
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h, err := NewHandler(HandlerParams{
		Logger:             zap.NewNop(),
		AgentURL:           "ws://127.0.0.1:1",
		BasePath:           basePath,
		MaxRequestBodySize: 1 << 20,
		EnableMCP:          true,
		QueryService:       svc,
		TenancyMgr:         tenancy.NewManager(&tenancy.Options{}),
		Telset:             telemetry.NoopSettings(),
	})
	require.NoError(t, err)
	return h
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

// TestNewHandlerSkillsDir confirms SkillsDir reaches the turn-scoped MCP
// endpoint's construction with the same two-tier failure handling as the
// shared endpoint: an unusable path fails NewHandler, but a malformed
// individual skill next to a valid one does not.
func TestNewHandlerSkillsDir(t *testing.T) {
	newParams := func(skillsDir string) HandlerParams {
		svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
		return HandlerParams{
			Logger:             zap.NewNop(),
			AgentURL:           "ws://127.0.0.1:1",
			BasePath:           "",
			MaxRequestBodySize: 1 << 20,
			EnableMCP:          true,
			QueryService:       svc,
			TenancyMgr:         tenancy.NewManager(&tenancy.Options{}),
			Telset:             telemetry.NoopSettings(),
			SkillsDir:          skillsDir,
		}
	}

	t.Run("valid dir succeeds", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, dir, "slow-db-call")
		h, err := NewHandler(newParams(dir))
		require.NoError(t, err)
		require.NotNil(t, h.mcp)
	})

	t.Run("missing dir path fails construction", func(t *testing.T) {
		_, err := NewHandler(newParams(filepath.Join(t.TempDir(), "no-such-dir")))
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("malformed skill inside the dir does not fail construction", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, dir, "good-skill")
		badPath := filepath.Join(dir, "bad-skill", "SKILL.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0o755))
		require.NoError(t, os.WriteFile(badPath, []byte("---\nname: MISMATCH\n---\nbody\n"), 0o600))

		h, err := NewHandler(newParams(dir))
		require.NoError(t, err, "one bad operator skill must not fail construction")
		require.NotNil(t, h.mcp)
	})
}

// writeSkill writes a minimal valid SKILL.md at <dir>/<name>/SKILL.md.
func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, name, "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	content := "---\nname: " + name + "\ndescription: A valid test skill.\n---\n\n# " + name + "\n"
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func TestRegisterRoutesOmitsMCPEndpointWhenDisabled(t *testing.T) {
	h, err := NewHandler(HandlerParams{Logger: zap.NewNop(), AgentURL: "ws://127.0.0.1:1", BasePath: "", MaxRequestBodySize: 1 << 20})
	require.NoError(t, err)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// With MCP disabled the route is never mounted, so any turn URL is a 404
	// regardless of whether a turn is active.
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ai/mcp/any-id/mcp", http.NoBody))
	assert.Equal(t, http.StatusNotFound, rr.Code, "turn-scoped MCP endpoint must not be mounted when disabled")
}
