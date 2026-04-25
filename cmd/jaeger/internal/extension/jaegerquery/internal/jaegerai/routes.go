// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"

	"go.uber.org/zap"
)

const (
	routeChat = "/api/ai/chat"
	routeMCP  = "/api/ai/mcp/{" + contextualMCPIDPathValue + "}"
)

// Handler is the entry point for the jaeger-query AI gateway. It owns the
// per-turn contextual tools store, the chat handler, and the per-session
// MCP endpoint, and registers all of them on the caller-provided mux.
//
// Callers construct a Handler once (in jaegerquery's Start path), then call
// RegisterRoutes when wiring the HTTP mux. This mirrors the APIHandler /
// HTTPGateway pattern used by sibling jaeger-query subsystems and keeps all
// AI dependencies inside the jaegerai package.
type Handler struct {
	logger             *zap.Logger
	store              *ContextualToolsStore
	agentURL           string
	basePath           string
	maxRequestBodySize int64
}

// NewHandler constructs a jaegerai.Handler with a freshly-allocated
// ContextualToolsStore. agentURL is the WebSocket endpoint of the ACP
// sidecar; basePath is the jaeger-query base path used to prefix the
// AI routes (and to build the absolute MCP URL the sidecar dials back).
// maxRequestBodySize bounds the chat request body to prevent abuse.
func NewHandler(logger *zap.Logger, agentURL, basePath string, maxRequestBodySize int64) *Handler {
	return &Handler{
		logger:             logger,
		store:              NewContextualToolsStore(),
		agentURL:           agentURL,
		basePath:           basePath,
		maxRequestBodySize: maxRequestBodySize,
	}
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux. The
// chat endpoint streams ACP turns to/from the sidecar; the MCP endpoint
// serves the per-turn contextual tools snapshot the sidecar dials back to
// pull AG-UI tools from.
func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	chatPath := routeChat
	mcpPath := routeMCP
	if h.basePath != "" && h.basePath != "/" {
		chatPath = h.basePath + chatPath
		mcpPath = h.basePath + mcpPath
	}
	router.HandleFunc(chatPath, NewChatHandler(h.logger, h.store, h.agentURL, h.basePath, h.maxRequestBodySize).ServeHTTP)
	router.Handle(mcpPath, NewContextualMCPHandler(h.logger, h.store))
}
