// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const routeChat = "/api/ai/chat"

// normalizeBasePath canonicalises the operator-supplied jaeger-query base
// path so route registration agrees on a single prefix. Empty and "/" both
// mean "no prefix" and are returned as "". Otherwise any trailing slash is
// trimmed so concatenating "/api/..." can never produce a double slash.
func normalizeBasePath(basePath string) string {
	if basePath == "" || basePath == "/" {
		return ""
	}
	return strings.TrimSuffix(basePath, "/")
}

// Handler is the entry point for the jaeger-query AI gateway. It owns the
// per-turn contextual tools store, the session-stream registry, and the chat
// handler, and registers them on the caller-provided mux.
//
// Callers construct a Handler once (in jaegerquery's Start path), then call
// RegisterRoutes when wiring the HTTP mux. This mirrors the APIHandler /
// HTTPGateway pattern used by sibling jaeger-query subsystems and keeps all
// AI dependencies inside the jaegerai package.
type Handler struct {
	logger             *zap.Logger
	store              *ContextualToolsStore
	streams            *sessionStreams
	agentURL           string
	basePath           string
	maxRequestBodySize int64
	// mcpHandler serves the session-scoped MCP endpoint. Non-nil only when the
	// operator enabled MCP (Deps.EnableMCP); otherwise the endpoint is not
	// mounted and the gateway advertises AI chat only.
	mcpHandler http.Handler
}

// Deps carries the dependencies for the AI gateway Handler. Grouping them in a
// struct keeps the constructor readable as the gateway gains MCP wiring
// (query service, tenancy, telemetry) on top of the chat parameters.
type Deps struct {
	Logger             *zap.Logger
	AgentURL           string
	BasePath           string
	MaxRequestBodySize int64
	// EnableMCP mounts the session-scoped telemetry MCP endpoint. When false,
	// only the chat endpoint is registered.
	EnableMCP    bool
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Telset       telemetry.Settings
}

// NewHandler constructs a jaegerai.Handler with a freshly-allocated
// ContextualToolsStore and sessionStreams. basePath is normalized once so the
// registered mux patterns use a single canonical prefix. When d.EnableMCP is
// set, the session-scoped MCP handler is built from the supplied query service,
// tenancy manager, and telemetry settings.
func NewHandler(d Deps) *Handler {
	basePath := normalizeBasePath(d.BasePath)
	h := &Handler{
		logger:             d.Logger,
		store:              NewContextualToolsStore(),
		streams:            newSessionStreams(),
		agentURL:           d.AgentURL,
		basePath:           basePath,
		maxRequestBodySize: d.MaxRequestBodySize,
	}
	if d.EnableMCP {
		telemetryHandler := mcptools.NewHandler(d.Telset, d.QueryService, d.TenancyMgr, mcptools.DefaultConfig())
		h.mcpHandler = &mcpSessionHandler{
			telemetryHandler: telemetryHandler,
			streams:          h.streams,
			basePath:         basePath,
			logger:           d.Logger,
		}
	}
	return h
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux:
//
//   - <basePath>/api/ai/chat              — streams ACP turns to/from the sidecar.
//   - <basePath>/api/ai/mcp/<id>[/...]    — session-scoped MCP endpoint (only
//     when MCP is enabled). Both the slash and no-slash forms are mounted; the
//     {sessionID} wildcard is more specific than the session-free
//     "/api/ai/mcp/" pattern jaeger-query mounts, so all coexist.
func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	chat := NewChatHandler(h.logger, h.store, h.agentURL, h.basePath, h.maxRequestBodySize)
	chat.streams = h.streams
	router.HandleFunc(h.basePath+routeChat, chat.ServeHTTP)

	if h.mcpHandler != nil {
		router.Handle(h.basePath+routeMCPSession, h.mcpHandler)
		router.Handle(h.basePath+routeMCPSessionNoSlash, h.mcpHandler)
	}
}
