// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
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

// turnScopedEndpoint serves the turn-scoped MCP endpoint. It advertises the UI
// tools the frontend declared for that turn — plus, when the operator enabled the
// telemetry MCP server, the built-in telemetry tools — and dispatches UI-tool calls
// back to the browser over the turn's SSE stream. Access is gated to route ids that
// belong to an active chat turn (present in turnRegistry).
type turnScopedEndpoint struct {
	// streamable is the closeable MCP streamable-HTTP handler (from
	// mcptools.WrapHTTP) serving a single shared server. Per-turn UI tools are
	// layered on by the uiToolsMiddleware registered on that server, keyed by the
	// route id carried in the request context. Its Close reaps the server's
	// sessions, so this endpoint's Close simply delegates to it.
	streamable *mcptools.Handler
	// server is the shared MCP server behind streamable. It is retained only so the
	// chat endpoint can gate its announcement on MCP being wired (see
	// chatEndpoint.announceMCP); teardown is delegated to streamable, not done here.
	server   *mcp.Server
	turns    *turnRegistry
	basePath string
	logger   *zap.Logger
}

// turnScopedEndpointBuilder collects the endpoint's dependencies. mcpConfig
// arrives ready-made: deciding MCP configuration belongs with the gateway's
// other config handling, not in this constructor.
type turnScopedEndpointBuilder struct {
	telset     telemetry.Settings
	queryAPI   *querysvc.QueryService
	tenancyMgr *tenancy.Manager
	turns      *turnRegistry
	basePath   string
	mcpConfig  mcptools.Config
	// enableMCP layers the built-in telemetry tools onto the server. When false,
	// the endpoint serves the turn's UI tools alone, so UI-tool dispatch does not
	// depend on ai.enable_mcp.
	enableMCP bool
}

// build assembles the turn-scoped handler around a single shared MCP server. The
// per-turn UI tools are layered on per-request by uiToolsMiddleware, so no server
// has to be stood up per turn. Whether the server also carries the built-in
// telemetry tools is gated by enableMCP; without them it serves UI tools alone.
func (b turnScopedEndpointBuilder) build() *turnScopedEndpoint {
	var srv *mcp.Server
	if b.enableMCP {
		srv = mcptools.NewServer(b.telset, b.queryAPI, b.mcpConfig)
	} else {
		srv = mcptools.NewServerWithoutTools(b.telset, b.mcpConfig)
	}
	srv.AddReceivingMiddleware(uiToolsMiddleware(b.turns, b.telset.Logger))
	return &turnScopedEndpoint{
		streamable: mcptools.WrapHTTP(srv, b.tenancyMgr, b.telset),
		server:     srv,
		turns:      b.turns,
		basePath:   b.basePath,
		logger:     b.telset.Logger,
	}
}

// registerRoutes mounts the endpoint on both the slash and no-slash forms of its
// URL. Keeping this here means the route patterns stay owned by this file rather
// than leaking into the gateway HTTP handler.
func (h *turnScopedEndpoint) registerRoutes(router *http.ServeMux) {
	router.Handle(h.basePath+routeTurnMCP, h)
	router.Handle(h.basePath+routeTurnMCPNoSlash, h)
}

var _ io.Closer = (*turnScopedEndpoint)(nil)

// Close reaps the MCP sessions still bound to this endpoint's server so they do not
// outlive it, delegating to the shared mcptools teardown (see mcptools.Handler.Close)
// so the shared and turn-scoped mounts tear down the same way.
func (h *turnScopedEndpoint) Close() error {
	return h.streamable.Close()
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
