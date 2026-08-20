// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
)

// routeMCPPrefix is the single source of truth for the endpoint's path: the mux
// patterns, the prefix ServeHTTP strips, and the URL announced to the sidecar
// (see chatEndpoint.announceMCP) all derive from it, so they cannot drift.
//
// routeTurnMCP and routeTurnMCPNoSlash are strictly more specific than the shared
// "/api/ai/mcp/" pattern jaeger-query mounts, so all three coexist on one mux:
//
//	/api/ai/mcp/           → shared handler (jaeger-query)
//	/api/ai/mcp/<id>       → turn-scoped (this handler)
//	/api/ai/mcp/<id>/...   → turn-scoped (this handler)
//
// Registering both the slash and no-slash forms is deliberate: without the
// no-slash pattern, a client dialing "/api/ai/mcp/<id>" (no trailing slash) would
// fall through to the shared subtree pattern instead of the turn-scoped handler.
const (
	routeMCPPrefix      = "/api/ai/mcp/"
	routeTurnMCPNoSlash = routeMCPPrefix + "{mcpRouteID}"
	routeTurnMCP        = routeTurnMCPNoSlash + "/"
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
	// streamable is the shared telemetry MCP endpoint's handler, borrowed from the
	// query server: this endpoint serves that same *mcp.Server, so the telemetry
	// tools are registered once for both mounts. Per-turn UI tools are layered on by
	// the uiToolsMiddleware build installs on it, keyed by the route id carried in
	// the request context. The query server owns the handler's teardown — this
	// endpoint only borrows it and closes nothing.
	streamable http.Handler
	turns      *turnRegistry
	basePath   string
	logger     *zap.Logger
}

// turnScopedEndpointBuilder collects the endpoint's dependencies. The MCP handler
// arrives ready-made because the shared telemetry endpoint exists independently of
// this gateway — external MCP clients use it with no chat sidecar involved — so the
// query server builds it and this endpoint layers the per-turn UI tools onto it.
type turnScopedEndpointBuilder struct {
	shared   *mcptools.Handler
	turns    *turnRegistry
	basePath string
	logger   *zap.Logger
}

// build layers this gateway's per-turn UI tools onto the shared MCP server and
// serves it under the turn-scoped routes. The telemetry tools are a fixed
// capability already registered on that server, and each turn's UI tools are added
// per-request by uiToolsMiddleware, so no server has to be stood up per turn — nor
// a second one for this mount.
func (b turnScopedEndpointBuilder) build() *turnScopedEndpoint {
	b.shared.AddReceivingMiddleware(uiToolsMiddleware(b.turns, b.logger))
	return &turnScopedEndpoint{
		streamable: b.shared,
		turns:      b.turns,
		basePath:   b.basePath,
		logger:     b.logger,
	}
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
	prefix := h.basePath + routeMCPPrefix + mcpRouteID
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	rewritten := r.Clone(context.WithValue(r.Context(), mcpRouteIDContextKey{}, mcpRouteID))
	rewritten.URL.Path = rest
	rewritten.URL.RawPath = ""
	h.streamable.ServeHTTP(w, rewritten)
}
