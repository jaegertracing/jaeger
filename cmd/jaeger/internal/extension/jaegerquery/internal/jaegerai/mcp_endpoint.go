// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// routeMCPSession and routeMCPSessionNoSlash are the session-scoped MCP
// patterns. Both are strictly more specific than the session-free
// "/api/ai/mcp/" pattern jaeger-query mounts, so all three coexist on one mux:
//
//	/api/ai/mcp/           → session-free handler (jaeger-query)
//	/api/ai/mcp/<id>       → session-scoped (this handler)
//	/api/ai/mcp/<id>/...   → session-scoped (this handler)
//
// Registering both the slash and no-slash forms is deliberate: without the
// no-slash pattern, a client dialing "/api/ai/mcp/<id>" (no trailing slash)
// would fall through to the session-free subtree pattern instead of reaching
// the session-scoped handler.
const (
	routeMCPSession        = "/api/ai/mcp/{sessionID}/"
	routeMCPSessionNoSlash = "/api/ai/mcp/{sessionID}"
)

// mcpSessionHandler serves the session-scoped MCP endpoint. It advertises the
// same telemetry tools as the session-free endpoint, but only for a session id
// that belongs to an active chat turn (registered in sessionStreams). Scoping
// to an active turn is what will let a later change layer that turn's UI tools
// on top; for now it gates access and returns telemetry tools.
type mcpSessionHandler struct {
	// telemetryHandler is the shared telemetry MCP handler (mcptools.NewHandler).
	// It is path-agnostic, so we strip the session prefix and hand it a clean
	// root.
	telemetryHandler http.Handler
	streams          *sessionStreams
	basePath         string
	logger           *zap.Logger
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
	// its own root. The no-slash form strips to "", which we normalize to "/".
	// Our routes carry no percent-encoding past the UUID, so Path is canonical
	// and RawPath is cleared.
	prefix := h.basePath + "/api/ai/mcp/" + sessionID
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	rewritten := r.Clone(r.Context())
	rewritten.URL.Path = rest
	rewritten.URL.RawPath = ""
	h.telemetryHandler.ServeHTTP(w, rewritten)
}
