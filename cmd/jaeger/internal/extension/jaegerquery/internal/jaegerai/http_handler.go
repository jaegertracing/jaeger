// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

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
	// mcp is the turn-scoped MCP endpoint. Non-nil only when the operator enabled
	// MCP (HandlerParams.EnableMCP); otherwise the endpoint is not mounted and the
	// gateway advertises AI chat only.
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
	// EnableMCP mounts the turn-scoped telemetry MCP endpoint. When false, only the
	// chat endpoint is registered.
	EnableMCP bool
	// MCPBaseURL is the scheme+authority (e.g. "https://jaeger.example.com:16686")
	// the gateway announces to the sidecar so it can dial the turn-scoped MCP
	// endpoint. Empty announces nothing — see announceMCPServers. Ignored when
	// EnableMCP is false.
	MCPBaseURL   string
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Telset       telemetry.Settings
}

// NewHandler constructs a jaegerai.Handler, building the endpoints it will mount.
// basePath is normalized once so the registered mux patterns use a single
// canonical prefix. The chat and turn-scoped MCP endpoints share one turnRegistry
// so a chat turn and its MCP callbacks resolve to the same turn. When p.EnableMCP
// is set, the turn-scoped MCP endpoint is built from the supplied query service,
// tenancy manager, and telemetry settings.
func NewHandler(p HandlerParams) *Handler {
	basePath := normalizeBasePath(p.BasePath)
	turns := newTurnRegistry()
	chat := newChatEndpoint(p.Logger, NewContextualToolsStore(), turns, p.AgentURL, basePath, p.MaxRequestBodySize)
	h := &Handler{basePath: basePath, chat: chat}
	if p.EnableMCP {
		h.mcp = newTurnScopedEndpoint(p.Telset, p.QueryService, p.TenancyMgr, turns, basePath, p.Logger)
		// Hand the chat endpoint the shared server and its reachable URL so each
		// turn announces this endpoint to the sidecar (see chatEndpoint.announceMCP).
		chat.mcpServer = h.mcp.server
		// TrimRight, not TrimSuffix: config only has to be an absolute URL, so a
		// value like "http://host:16686//" is legal, and leaving either slash on
		// would announce "…//api/ai/mcp/<id>/" — a path the mux never matches.
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

var _ io.Closer = (*Handler)(nil)

// Close tears down any MCP sessions still bound to the shared server so they do
// not outlive the gateway. The go-sdk reaps a session only when it goes idle
// (see StreamableHTTPOptions.SessionTimeout), so a turn whose sidecar has not
// disconnected would otherwise linger after Shutdown. Called by the jaeger-query
// server's Close path (Server.Close → httpServer.Close → closeAll → here).
//
// ServerSession.Close is the only teardown the SDK exposes — there is no
// server-level Shutdown. Sessions() yields a snapshot (it clones under lock), so
// closing each one mid-iteration, which deregisters it, is safe.
//
// A nil Handler is what jaeger-query holds when the AI gateway is disabled, and a
// Handler with no MCP server is what it holds when only chat is enabled — both
// close to nothing, so callers need no guard.
func (h *Handler) Close() error {
	if h == nil || h.mcp == nil {
		return nil
	}
	var errs []error
	for session := range h.mcp.server.Sessions() {
		errs = append(errs, session.Close())
	}
	return errors.Join(errs...)
}
