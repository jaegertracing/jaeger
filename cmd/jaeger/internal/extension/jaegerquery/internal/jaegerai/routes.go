// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

const routeChat = "/api/ai/chat"

// McpServerConfig is the operator-supplied MCP server entry the gateway
// forwards to the agent via NewSessionRequest.mcpServers on every chat
// turn. Only HTTP transport is exposed today (Jaeger MCP is HTTP); the
// type is kept in the jaegerai package so the config layer in flags.go
// can reference it without dragging the ACP SDK into the public schema.
//
// Agents that advertise mcp_capabilities.http (e.g. claude-agent-acp)
// will connect to each entry and expose its tools to the model. Agents
// that ignore wire-pushed MCP (e.g. the Gemini sidecar, which reads its
// own JAEGER_MCP_URL env) silently drop these entries — both paths are
// safe to leave configured.
type McpServerConfig struct {
	// Name is the local handle the agent uses to namespace this server's
	// tools (e.g. Claude exposes them as mcp__<name>__<tool>).
	Name string `mapstructure:"name" valid:"required"`
	// URL is the HTTP MCP endpoint (e.g. http://localhost:16687/mcp).
	URL string `mapstructure:"url" valid:"required"`
}

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
// per-turn contextual tools store and the chat handler, and registers them
// on the caller-provided mux.
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
	mcpServers         []McpServerConfig
}

// NewHandler constructs a jaegerai.Handler with a freshly-allocated
// ContextualToolsStore. agentURL is the WebSocket endpoint of the ACP
// sidecar; basePath is the jaeger-query base path used to prefix the AI
// routes. maxRequestBodySize bounds the chat request body to prevent abuse.
// mcpServers may be nil or empty when no MCP entries should be advertised
// to the agent; otherwise it carries the operator-configured list.
//
// basePath is normalized once so the registered mux pattern uses a single
// canonical prefix.
func NewHandler(logger *zap.Logger, agentURL, basePath string, maxRequestBodySize int64, mcpServers []McpServerConfig) *Handler {
	return &Handler{
		logger:             logger,
		store:              NewContextualToolsStore(),
		agentURL:           agentURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
		mcpServers:         mcpServers,
	}
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux. The
// chat endpoint streams ACP turns to/from the sidecar.
func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc(h.basePath+routeChat, NewChatHandler(h.logger, h.store, h.agentURL, h.basePath, h.maxRequestBodySize, h.mcpServers).ServeHTTP)
}

// buildMcpServersWire translates the operator-supplied McpServerConfig
// entries into the ACP wire shape (HTTP McpServer variants). Returned
// slice is always non-nil so the SDK marshals it as `[]` even when no
// servers are configured. Each entry's Headers slice is also non-nil for
// the same reason: ACP forbids null where the schema requires an array.
func buildMcpServersWire(servers []McpServerConfig) []acp.McpServer {
	out := make([]acp.McpServer, 0, len(servers))
	for _, ms := range servers {
		out = append(out, acp.McpServer{
			Http: &acp.McpServerHttpInline{
				Name:    ms.Name,
				Url:     ms.URL,
				Headers: []acp.HttpHeader{},
			},
		})
	}
	return out
}
