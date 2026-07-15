// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
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
	logger *zap.Logger
	// store and turns are two per-turn registries that are separate only during
	// this transition, because they're keyed differently: store by the ACP session
	// id (read by the ext-method UI-tool path, retired in a later milestone), turns
	// by the gateway-minted route id in the turn-scoped MCP URL. Once the ext-method
	// path is retired they collapse into one.
	store              *ContextualToolsStore
	turns              *turnRegistry
	agentURL           string
	basePath           string
	maxRequestBodySize int64
	// mcpHandler serves the turn-scoped MCP endpoint. Non-nil only when the operator
	// enabled MCP (HandlerParams.EnableMCP); otherwise the endpoint is not mounted
	// and the gateway advertises AI chat only.
	mcpHandler http.Handler
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
	EnableMCP    bool
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Telset       telemetry.Settings
}

// NewHandler constructs a jaegerai.Handler with a freshly-allocated
// ContextualToolsStore and turnRegistry. basePath is normalized once so the
// registered mux patterns use a single canonical prefix. When p.EnableMCP is set,
// the turn-scoped MCP endpoint is built from the supplied query service, tenancy
// manager, and telemetry settings.
func NewHandler(p HandlerParams) *Handler {
	basePath := normalizeBasePath(p.BasePath)
	h := &Handler{
		logger:             p.Logger,
		store:              NewContextualToolsStore(),
		turns:              newTurnRegistry(),
		agentURL:           p.AgentURL,
		basePath:           basePath,
		maxRequestBodySize: p.MaxRequestBodySize,
	}
	if p.EnableMCP {
		h.mcpHandler = newTurnScopedEndpoint(p.Telset, p.QueryService, p.TenancyMgr, h.turns, basePath, p.Logger)
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
	chat := newChatEndpoint(h.logger, h.store, h.agentURL, h.basePath, h.maxRequestBodySize)
	chat.turns = h.turns
	router.HandleFunc(h.basePath+routeChat, chat.ServeHTTP)

	if h.mcpHandler != nil {
		router.Handle(h.basePath+routeMCPSession, h.mcpHandler)
		router.Handle(h.basePath+routeMCPSessionNoSlash, h.mcpHandler)
	}
}
