// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"

	"go.uber.org/zap"
)

// routeMCPSession is the session-scoped MCP path. The trailing {sessionID}
// wildcard makes it strictly more specific than the session-free
// "/api/ai/mcp/" pattern jaeger-query mounts, so the two coexist on the same
// mux without conflict: a bare "/api/ai/mcp/" hits the session-free handler,
// while "/api/ai/mcp/<id>/..." hits this one.
const routeMCPSession = "/api/ai/mcp/{sessionID}/"

// mcpSessionHandler serves the session-scoped MCP endpoint. It advertises the
// same telemetry tools as the session-free endpoint, but only for a session id
// that belongs to an active chat turn (registered in sessionStreams). Scoping
// to an active turn is what will let a later change layer that turn's UI tools
// on top; for now it gates access and returns telemetry tools.
type mcpSessionHandler struct {
	// telemetry is the shared telemetry MCP handler (mcptools.NewHandler). It is
	// path-agnostic, so we strip the session prefix and hand it a clean root.
	telemetry http.Handler
	streams   *sessionStreams
	basePath  string
	logger    *zap.Logger
}

func (h *mcpSessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if h.streams.get(sessionID) == nil {
		// Unknown or expired session id: the scoped endpoint is only valid
		// during an active chat turn, so a missing entry is a client error.
		h.logger.Debug("session-scoped MCP request for unknown session", zap.String("session_id", sessionID))
		http.NotFound(w, r)
		return
	}
	// Strip "<basePath>/api/ai/mcp/<sessionID>" so the wrapped MCP handler sees
	// its own root (matching how the session-free endpoint is mounted).
	prefix := h.basePath + "/api/ai/mcp/" + sessionID
	http.StripPrefix(prefix, h.telemetry).ServeHTTP(w, r)
}
