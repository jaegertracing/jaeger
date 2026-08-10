// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/internal/telemetry"
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
	// mcp is the turn-scoped MCP endpoint. Non-nil only when the caller supplied an
	// MCP handler (HandlerParams.MCP); otherwise the endpoint is not mounted and the
	// gateway advertises AI chat only.
	mcp *turnScopedEndpoint
}

// HandlerParams carries the dependencies for the AI gateway Handler. Grouping them
// in a struct keeps the constructor readable as the gateway gains MCP wiring on top
// of the chat parameters.
type HandlerParams struct {
	Logger             *zap.Logger
	AgentURL           string
	BasePath           string
	MaxRequestBodySize int64
	// MCP is the shared telemetry MCP endpoint, already built and mounted by the
	// query server because it serves external MCP clients whether or not this gateway
	// exists. Supplying it mounts the turn-scoped endpoint on the same *mcp.Server,
	// with this gateway's per-turn UI tools layered on; nil means the operator did not
	// configure ai.mcp, so only the chat endpoint is registered. The query server owns
	// its teardown — this gateway only borrows it.
	MCP *mcptools.Handler
	// MCPBaseURL is the scheme+authority (e.g. "https://jaeger.example.com:16686")
	// the gateway announces to the sidecar so it can dial the turn-scoped MCP
	// endpoint. Empty announces nothing — see chatEndpoint.announceMCP. Ignored when
	// MCP is nil.
	MCPBaseURL string
	Telset     telemetry.Settings
}

// NewHandler constructs a jaegerai.Handler, building the endpoints it will mount.
// basePath is normalized once so the registered mux patterns use a single
// canonical prefix. The chat and turn-scoped MCP endpoints share one turnRegistry
// so a chat turn and its MCP callbacks resolve to the same turn. When p.MCP is
// supplied, the turn-scoped MCP endpoint is layered onto that shared server.
func NewHandler(p HandlerParams) *Handler {
	basePath := normalizeBasePath(p.BasePath)
	turns := newTurnRegistry()
	chat := newChatEndpoint(p.Logger, NewContextualToolsStore(), turns, p.AgentURL, basePath, p.MaxRequestBodySize)
	h := &Handler{basePath: basePath, chat: chat}
	if p.MCP != nil {
		h.mcp = turnScopedEndpointBuilder{
			shared:   p.MCP,
			turns:    turns,
			basePath: basePath,
			logger:   p.Telset.Logger,
		}.build()
		// Hand the chat endpoint the endpoint's reachable base URL so each turn
		// announces it to the sidecar (see chatEndpoint.announceMCP). Setting it only
		// here is what keeps the announcement off when no endpoint is mounted.
		//
		// TrimRight, not TrimSuffix: config only has to be an absolute URL, so a
		// value like "http://host:16686//" is legal, and TrimSuffix would leave one
		// slash on. A doubled slash still reaches the endpoint — ServeMux redirects
		// to the cleaned path — but at the cost of a redirect on every MCP call.
		chat.mcpBaseURL = strings.TrimRight(p.MCPBaseURL, "/")
	}
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

// The gateway holds nothing past the request that opened it: the MCP sessions its
// turn-scoped endpoint binds live on the shared server the query server built and
// closes (see mcptools.Handler.Close), so there is nothing here to tear down.
