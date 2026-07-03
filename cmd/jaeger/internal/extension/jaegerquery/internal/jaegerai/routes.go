// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"
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
