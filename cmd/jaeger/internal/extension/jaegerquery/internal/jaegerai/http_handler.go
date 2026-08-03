// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const routeChat = "/api/ai/chat"

// Handler is the gateway's HTTP handler and the entry point for the jaeger-query
// AI gateway. It owns the turn registry and the endpoints (chat and, when MCP is
// enabled, the turn-scoped MCP endpoint), and mounts them on the caller-provided
// mux (see RegisterRoutes).
//
// Callers construct a Handler once (in jaegerquery's Start path), then call
// RegisterRoutes when wiring the HTTP mux. This mirrors the APIHandler /
// HTTPGateway pattern used by sibling jaeger-query subsystems and keeps all AI
// dependencies inside the jaegerai package.
type Handler struct {
	basePath string
	// chat is the chat endpoint (/api/ai/chat), always present.
	chat *chatEndpoint
	// mcp is the turn-scoped MCP endpoint. Always built for a chat gateway (it
	// carries the turn's UI tools); HandlerParams.EnableMCP only controls whether it
	// also serves the built-in telemetry tools.
	mcp *turnScopedEndpoint
}

// HandlerParams carries the dependencies for the AI gateway Handler. Grouping them
// in a struct keeps the constructor readable as the gateway gains MCP wiring (query
// service, tenancy, telemetry) on top of the chat parameters.
type HandlerParams struct {
	Logger             *zap.Logger
	AgentURL           string
	BasePath           string
	MaxRequestBodySize int64
	// EnableMCP layers the built-in telemetry tools onto the turn-scoped endpoint
	// (and, via the query server, mounts the external shared endpoint). It does not
	// gate the turn-scoped endpoint itself, which always mounts for a chat gateway to
	// carry the turn's UI tools.
	EnableMCP bool
	// MCPBaseURL is the scheme+authority (e.g. "https://jaeger.example.com:16686")
	// the gateway announces to the sidecar so it can dial the turn-scoped MCP
	// endpoint. Empty announces nothing — see chatEndpoint.announceMCP. Announced
	// whenever set, since the turn-scoped endpoint exists regardless of EnableMCP.
	MCPBaseURL   string
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Telset       telemetry.Settings
	// MCPConfig configures the MCP server behind the turn-scoped endpoint. The
	// caller passes the same value it gives the shared endpoint, so the two
	// cannot drift. Read only when EnableMCP is set.
	MCPConfig mcptools.Config
}

// NewHandler constructs a jaegerai.Handler, building the endpoints it will mount.
// basePath is normalized once so the registered mux patterns use a single
// canonical prefix. The chat and turn-scoped MCP endpoints share one turnRegistry
// so a chat turn and its MCP callbacks resolve to the same turn.
//
// The turn-scoped MCP endpoint is always built: it carries the turn's UI tools,
// which the sidecar dispatches over MCP independently of ai.enable_mcp. p.EnableMCP
// only decides whether the built-in telemetry tools are layered onto it too (see
// newTurnScopedEndpoint).
func NewHandler(p HandlerParams) *Handler {
	basePath := normalizeBasePath(p.BasePath)
	turns := newTurnRegistry()
	chat := newChatEndpoint(p.Logger, NewContextualToolsStore(), turns, p.AgentURL, basePath, p.MaxRequestBodySize)
	h := &Handler{basePath: basePath, chat: chat}
	// The turn-scoped endpoint always mounts for a chat gateway: it carries the
	// turn's UI tools, which the sidecar dispatches over MCP independently of
	// ai.enable_mcp. enableMCP only decides whether the built-in telemetry tools are
	// layered onto it too (see turnScopedEndpointBuilder.build).
	h.mcp = turnScopedEndpointBuilder{
		telset:     p.Telset,
		queryAPI:   p.QueryService,
		tenancyMgr: p.TenancyMgr,
		turns:      turns,
		basePath:   basePath,
		mcpConfig:  p.MCPConfig,
		enableMCP:  p.EnableMCP,
	}.build()
	// Hand the chat endpoint the endpoint's reachable base URL so each turn
	// announces it to the sidecar (see chatEndpoint.announceMCP). The endpoint
	// always exists now, so announcement is gated only by the base URL resolving to
	// a non-empty value, not by enable_mcp.
	//
	// TrimRight, not TrimSuffix: config only has to be an absolute URL, so a value
	// like "http://host:16686//" is legal, and TrimSuffix would leave one slash on.
	// A doubled slash still reaches the endpoint — ServeMux redirects to the cleaned
	// path — but at the cost of a redirect on every MCP call.
	chat.mcpBaseURL = strings.TrimRight(p.MCPBaseURL, "/")
	return h
}

// normalizeBasePath canonicalises the operator-supplied jaeger-query base path so
// route registration agrees on a single prefix. Empty and "/" both mean "no prefix"
// and are returned as "". Otherwise any trailing slash is trimmed so concatenating
// "/api/..." can never produce a double slash.
func normalizeBasePath(basePath string) string {
	if basePath == "" || basePath == "/" {
		return ""
	}
	return strings.TrimSuffix(basePath, "/")
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux:
//
//   - <basePath>/api/ai/chat              — streams ACP turns to/from the sidecar.
//   - <basePath>/api/ai/mcp/<id>[/...]    — turn-scoped MCP endpoint (only when MCP
//     is enabled). Both the slash and no-slash forms are mounted; the {mcpRouteID}
//     wildcard is more specific than the shared "/api/ai/mcp/" pattern jaeger-query
//     mounts, so all coexist.
func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc(h.basePath+routeChat, h.chat.ServeHTTP)
	if h.mcp != nil {
		h.mcp.registerRoutes(router)
	}
}

// SharedMCPHandler returns a handler that serves this gateway's MCP server as the
// shared (turn-less) telemetry endpoint — the very server that backs the turn-scoped
// endpoint. The uiToolsMiddleware degrades to telemetry-only when a request carries
// no turn (external clients on the shared mount never get a route id stamped), so one
// server serves both mounts. The query server mounts this at /api/ai/mcp/ instead of
// standing up a second server (M7); it is reaped by this Handler's Close. The
// turn-scoped endpoint is always built for a chat gateway, so h.mcp is non-nil here.
func (h *Handler) SharedMCPHandler() http.Handler {
	return h.mcp.streamable
}

var _ io.Closer = (*Handler)(nil)

// Close shuts down the endpoints that hold resources past the request that opened
// them. Called by the jaeger-query server's Close path (Server.Close →
// httpServer.Close → closers.Close → here), so the gateway's MCP sessions do not
// outlive the server that served them.
//
// A nil Handler is what jaeger-query holds when the AI gateway is disabled — it
// closes to nothing, so callers need no guard.
func (h *Handler) Close() error {
	if h == nil || h.mcp == nil {
		return nil
	}
	return h.mcp.Close()
}
