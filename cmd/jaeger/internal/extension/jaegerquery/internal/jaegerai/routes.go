// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
)

const (
	routeChat = "/api/ai/chat"
	routeMCP  = "/api/ai/mcp/{" + contextualMCPIDPathValue + "}"
)

// normalizeBasePath canonicalises the operator-supplied jaeger-query base
// path so route registration and URL construction agree on the prefix.
// Empty and "/" both mean "no prefix" and are returned as "". Otherwise
// any trailing slash is trimmed so concatenating "/api/..." can never
// produce a double slash.
func normalizeBasePath(basePath string) string {
	if basePath == "" || basePath == "/" {
		return ""
	}
	return strings.TrimSuffix(basePath, "/")
}

// Handler is the entry point for the jaeger-query AI gateway. It owns the
// per-turn contextual tools store, the chat handler, and the per-turn
// contextual MCP endpoint, and registers all of them on the caller-provided
// mux.
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
//
// basePath is normalized once here so both the registered mux patterns and
// the contextual MCP URL embedded in NewSessionRequest.McpServers agree on
// a single canonical prefix.
func NewHandler(logger *zap.Logger, agentURL, basePath string, maxRequestBodySize int64) *Handler {
	return &Handler{
		logger:             logger,
		store:              NewContextualToolsStore(),
		agentURL:           agentURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
	}
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux. The
// chat endpoint streams ACP turns to/from the sidecar; the MCP endpoint
// serves the per-turn contextual tools snapshot the sidecar dials back to
// pull AG-UI tools from.
func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc(h.basePath+routeChat, NewChatHandler(h.logger, h.store, h.agentURL, h.basePath, h.maxRequestBodySize).ServeHTTP)
	router.Handle(h.basePath+routeMCP, NewContextualMCPHandler(h.logger, h.store))
}
