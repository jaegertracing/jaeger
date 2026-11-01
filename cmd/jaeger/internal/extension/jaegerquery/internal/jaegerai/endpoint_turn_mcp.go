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

	// mcpSessionIDHeader is the Streamable HTTP header that names an MCP session.
	mcpSessionIDHeader = "Mcp-Session-Id"
)

// mcpRouteIDContextKey carries the URL route id from ServeHTTP into the
// UI-dispatch middleware. ServeHTTP stamps it on the request context before
// delegating to the MCP handler; the go-sdk propagates the initialize
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
	// handler is this endpoint's own HTTP handler over the MCP server behind
	// baseHandler, which jaeger-query mounts at /api/ai/mcp/ and owns. Sharing the
	// server registers the telemetry tools once; having a handler of its own gives
	// this endpoint a session table of its own, so a turn's MCP sessions are
	// addressable only here, behind the token check, and never by session id
	// through the shared path.
	handler  http.Handler
	turns    *turnRegistry
	basePath string
	logger   *zap.Logger
}

// turnScopedEndpointBuilder collects the endpoint's dependencies. The MCP handler
// arrives ready-made: external MCP clients dial the shared telemetry endpoint with
// no chat sidecar involved, so the query server builds it and this endpoint layers
// the per-turn UI tools onto it.
type turnScopedEndpointBuilder struct {
	baseHandler *mcptools.Handler
	turns       *turnRegistry
	basePath    string
	logger      *zap.Logger
}

// build layers this gateway's per-turn UI tools onto the shared MCP server and
// serves it under the turn-scoped routes. uiToolsMiddleware adds each turn's UI
// tools per request, keyed by the route id in the request context, so neither a
// server per turn nor a second server for this mount is needed.
func (b turnScopedEndpointBuilder) build() *turnScopedEndpoint {
	b.baseHandler.AddReceivingMiddleware(uiToolsMiddleware(b.turns, b.logger))
	return &turnScopedEndpoint{
		handler:  b.baseHandler.NewIsolatedHandler(),
		turns:    b.turns,
		basePath: b.basePath,
		logger:   b.logger,
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
	turn := h.turns.get(mcpRouteID)
	if turn == nil {
		// Unknown or expired route id: the scoped endpoint is only valid
		// during an active chat turn, so a missing entry is a client error.
		h.logger.Debug("turn-scoped MCP request for unknown route id", zap.String("mcp_route_id", mcpRouteID))
		http.NotFound(w, r)
		return
	}
	// The route id names the turn; the token proves the caller was given it.
	// Anyone who can reach the query port can send a request to this path, so
	// the id alone, which travels in URLs and lands in logs, is not enough.
	if !turn.authorize(r.Header.Get(turnTokenHeader)) {
		h.logger.Debug("turn-scoped MCP request without the turn's token", zap.String("mcp_route_id", mcpRouteID))
		http.Error(w, "missing or invalid turn token", http.StatusUnauthorized)
		return
	}
	// A session id must belong to this turn. The transport resolves a session
	// by its id alone and serves it from the context it was created with, so a
	// session opened under another turn would run as that turn's caller.
	if sessionID := r.Header.Get(mcpSessionIDHeader); sessionID != "" && !turn.ownsSession(sessionID) {
		h.logger.Debug("turn-scoped MCP request for a session of another turn", zap.String("mcp_route_id", mcpRouteID))
		http.NotFound(w, r)
		return
	}
	// Strip "<basePath>/api/ai/mcp/<mcpRouteID>" so the wrapped MCP handler sees
	// its own root, and carry the id forward in the context for the UI-dispatch
	// middleware. The no-slash form strips to "", which we normalize to "/". Our
	// routes carry no percent-encoding past the UUID, so Path is canonical and
	// RawPath cleared.
	//
	// The request also runs as the chat caller (see turnCaller and turnState).
	prefix := h.basePath + routeMCPPrefix + mcpRouteID
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	ctx := turn.caller.contextFor(r.Context())
	rewritten := r.Clone(context.WithValue(ctx, mcpRouteIDContextKey{}, mcpRouteID))
	rewritten.URL.Path = rest
	rewritten.URL.RawPath = ""
	h.handler.ServeHTTP(w, rewritten)
}
