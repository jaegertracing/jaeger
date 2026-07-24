// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// routeTurnMCP and routeTurnMCPNoSlash are the turn-scoped MCP
// patterns. Both are strictly more specific than the shared
// "/api/ai/mcp/" pattern jaeger-query mounts, so all three coexist on one mux:
//
//	/api/ai/mcp/           → shared handler (jaeger-query)
//	/api/ai/mcp/<id>       → turn-scoped (this handler)
//	/api/ai/mcp/<id>/...   → turn-scoped (this handler)
//
// Registering both the slash and no-slash forms is deliberate: without the
// no-slash pattern, a client dialing "/api/ai/mcp/<id>" (no trailing slash)
// would fall through to the shared subtree pattern instead of reaching
// the turn-scoped handler.
const (
	routeTurnMCP        = "/api/ai/mcp/{mcpRouteID}/"
	routeTurnMCPNoSlash = "/api/ai/mcp/{mcpRouteID}"
)

// mcpRouteIDContextKey carries the URL route id from ServeHTTP into the
// UI-dispatch middleware. ServeHTTP stamps it on the request context before
// delegating to the streamable handler; the go-sdk propagates the initialize
// request's context values onto the resulting ServerSession, so every later
// tools/list and tools/call for that turn can recover the id here.
type mcpRouteIDContextKey struct{}

// mcpRouteIDFromContext returns the URL route id stamped by ServeHTTP, or ""
// when absent (e.g. a request that never went through ServeHTTP).
func mcpRouteIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(mcpRouteIDContextKey{}).(string)
	return id
}

// turnScopedEndpoint serves the turn-scoped MCP endpoint. It advertises the
// built-in telemetry tools plus the UI tools the frontend declared for that
// turn, and dispatches UI-tool calls back to the browser over the turn's
// SSE stream. Access is gated to route ids that belong to an active chat turn
// (present in turnRegistry).
type turnScopedEndpoint struct {
	// streamable is the MCP streamable-HTTP handler (from mcptools.WrapHTTP)
	// serving a single shared telemetry server. Per-turn UI tools are layered
	// on by the uiToolsMiddleware registered on that server, keyed by the
	// route id carried in the request context.
	streamable http.Handler
	turns      *turnRegistry
	basePath   string
	logger     *zap.Logger
}

// newTurnScopedEndpoint builds the turn-scoped handler around a single shared
// MCP server. The telemetry tools are a fixed capability, so they are registered
// once; the per-turn UI tools are layered on via uiToolsMiddleware, which
// reads the route id from the request context and, for that turn,
// advertises its UI tools in tools/list and dispatches their tools/call to the
// browser stream. This avoids standing up a fresh server per turn.
// skillsDir threads the operator's ai.skills_dir into the server so this
// endpoint serves the same merged skill tree as the shared one; an unusable
// skillsDir path fails construction.
func newTurnScopedEndpoint(telset telemetry.Settings, queryAPI *querysvc.QueryService, tenancyMgr *tenancy.Manager, turns *turnRegistry, basePath string, skillsDir string, logger *zap.Logger) (*turnScopedEndpoint, error) {
	cfg := mcptools.DefaultConfig()
	cfg.SkillsDir = skillsDir
	srv, err := mcptools.NewServer(telset, queryAPI, cfg)
	if err != nil {
		return nil, err
	}
	srv.AddReceivingMiddleware(uiToolsMiddleware(turns, logger))
	return &turnScopedEndpoint{
		streamable: mcptools.WrapHTTP(srv, tenancyMgr, telset),
		turns:      turns,
		basePath:   basePath,
		logger:     logger,
	}, nil
}

// registerRoutes mounts the endpoint on both the slash and no-slash forms of its
// URL. Keeping this here means the route patterns stay owned by this file rather
// than leaking into the gateway HTTP handler.
func (h *turnScopedEndpoint) registerRoutes(router *http.ServeMux) {
	router.Handle(h.basePath+routeTurnMCP, h)
	router.Handle(h.basePath+routeTurnMCPNoSlash, h)
}

func (h *turnScopedEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mcpRouteID := r.PathValue("mcpRouteID")
	if h.turns.get(mcpRouteID) == nil {
		// Unknown or expired route id: the scoped endpoint is only valid
		// during an active chat turn, so a missing entry is a client error.
		h.logger.Debug("turn-scoped MCP request for unknown route id", zap.String("mcp_route_id", mcpRouteID))
		http.NotFound(w, r)
		return
	}
	// Strip "<basePath>/api/ai/mcp/<mcpRouteID>" so the wrapped MCP handler sees
	// its own root, and carry the id forward in the context for the UI-dispatch
	// middleware. The no-slash form strips to "", which we normalize to "/". Our
	// routes carry no percent-encoding past the UUID, so Path is canonical and
	// RawPath cleared.
	prefix := h.basePath + "/api/ai/mcp/" + mcpRouteID
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	rewritten := r.Clone(context.WithValue(r.Context(), mcpRouteIDContextKey{}, mcpRouteID))
	rewritten.URL.Path = rest
	rewritten.URL.RawPath = ""
	h.streamable.ServeHTTP(w, rewritten)
}
