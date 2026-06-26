// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
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
// per-turn contextual tools store and the chat handler, and registers them
// on the caller-provided mux.
//
// Callers construct a Handler once (in jaegerquery's Start path), then call
// RegisterRoutes when wiring the HTTP mux. This mirrors the APIHandler /
// HTTPGateway pattern used by sibling jaeger-query subsystems and keeps all
// AI dependencies inside the jaegerai package.
type Handler struct {
	logger             *zap.Logger
	tracerProvider     trace.TracerProvider
	store              *ContextualToolsStore
	streams            *SessionStreams
	agentURL           string
	basePath           string
	maxRequestBodySize int64
	// mcpProxy is set by RegisterRoutes when the MCP route is mounted.
	// Kept on the Handler so the upstream MCP client session can be
	// released via Handler.Close — useful in tests and a hook for the
	// future extension Shutdown wiring.
	mcpProxy *MCPProxy
}

// NewHandler constructs a jaegerai.Handler with a freshly-allocated
// ContextualToolsStore and SessionStreams. agentURL is the WebSocket
// endpoint of the ACP sidecar; basePath is the jaeger-query base path
// used to prefix the AI routes. maxRequestBodySize bounds the chat
// request body to prevent abuse.
//
// tracerProvider is wired into the MCP proxy so dispatch_ui_tool and
// forward_upstream spans appear alongside jaeger_mcp's tool-level
// spans. Pass nil to disable tracing — useful in tests.
//
// basePath is normalized once so the registered mux pattern uses a single
// canonical prefix.
func NewHandler(logger *zap.Logger, tracerProvider trace.TracerProvider, agentURL, basePath string, maxRequestBodySize int64) *Handler {
	if tracerProvider == nil {
		tracerProvider = nooptrace.NewTracerProvider()
	}
	return &Handler{
		logger:             logger,
		tracerProvider:     tracerProvider,
		store:              NewContextualToolsStore(),
		streams:            NewSessionStreams(),
		agentURL:           agentURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
	}
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux.
//
//   - /api/ai/chat         — streams ACP turns to/from the sidecar.
//   - /api/ai/mcp/<uuid>/  — MCP server endpoint each ACP agent dials
//     as its single MCP egress. The chat handler mints a fresh uuid
//     per turn and embeds it in the URL announced via mcpServers; the
//     proxy resolves the uuid to the AG-UI session id on every
//     request so per-session UI tools are dispatched back to the
//     correct browser SSE stream. (See mcp_proxy.go for the dispatch
//     model.)
//
// The MCP route is mounted with a trailing slash so it acts as a prefix
// match; the proxy peels the uuid off the next path segment and hands
// the remainder to the wrapped streamable handler.
func (h *Handler) RegisterRoutes(ctx context.Context, router *http.ServeMux) {
	h.mcpProxy = NewMCPProxy(ctx, h.logger, h.tracerProvider, h.basePath, h.store, h.streams)
	router.Handle(h.basePath+routeMCPPrefix, h.mcpProxy)

	// Chat handler must learn about the proxy AFTER it's constructed
	// so the dispatcher can route mcp/connect, mcp/disconnect, and
	// mcp/message into the proxy's shared dispatch logic.
	chat := NewChatHandler(h.logger, h.store, h.agentURL, h.basePath, h.maxRequestBodySize).
		withSessionStreams(h.streams).
		withMCPProxy(h.mcpProxy)
	router.HandleFunc(h.basePath+routeChat, chat.ServeHTTP)
}

// Close releases the resources owned by the Handler — specifically the
// upstream MCP client session the proxy holds. Safe to call when
// RegisterRoutes was never invoked (mcpProxy is nil) and after the
// process has already torn down its sockets.
func (h *Handler) Close() error {
	if h.mcpProxy == nil {
		return nil
	}
	return h.mcpProxy.Close()
}
