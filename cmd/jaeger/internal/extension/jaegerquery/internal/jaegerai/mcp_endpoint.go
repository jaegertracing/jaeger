// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
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

// sessionIDContextKey carries the URL session id from ServeHTTP into the
// getServer callback, which runs inside the wrapped streamable handler after
// the path prefix has been stripped.
type sessionIDContextKey struct{}

// mcpSessionHandler serves the session-scoped MCP endpoint. It advertises the
// built-in telemetry tools plus the UI tools the frontend declared for that
// session, and dispatches UI-tool calls back to the browser over the session's
// SSE stream. Access is gated to session ids that belong to an active chat turn
// (present in sessionStreams).
type mcpSessionHandler struct {
	// streamable is the MCP streamable-HTTP handler (from mcptools.WrapHTTP)
	// whose getServer builds a fresh server per session: the telemetry tools
	// plus that session's UI tools.
	streamable http.Handler
	streams    *sessionStreams
	basePath   string
	logger     *zap.Logger
}

// newMCPSessionHandler builds the session-scoped handler. The getServer closure
// captures the telemetry dependencies and, per request, layers the looked-up
// session's UI tools on top of a fresh telemetry server.
func newMCPSessionHandler(telset telemetry.Settings, queryAPI *querysvc.QueryService, tenancyMgr *tenancy.Manager, streams *sessionStreams, basePath string, logger *zap.Logger) *mcpSessionHandler {
	h := &mcpSessionHandler{streams: streams, basePath: basePath, logger: logger}
	h.streamable = mcptools.WrapHTTP(func(r *http.Request) *mcp.Server {
		sessionID, _ := r.Context().Value(sessionIDContextKey{}).(string)
		srv := mcptools.NewServer(telset, queryAPI, mcptools.DefaultConfig())
		// sess is normally non-nil (ServeHTTP validates before delegating); a
		// nil here (session ended mid-request) degrades to telemetry-only.
		if sess := streams.get(sessionID); sess != nil {
			addUITools(srv, sess, logger)
		}
		return srv
	}, tenancyMgr, telset)
	return h
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
	// its own root, and carry the id forward in the context for getServer. The
	// no-slash form strips to "", which we normalize to "/". Our routes carry no
	// percent-encoding past the UUID, so Path is canonical and RawPath cleared.
	prefix := h.basePath + "/api/ai/mcp/" + sessionID
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	rewritten := r.Clone(context.WithValue(r.Context(), sessionIDContextKey{}, sessionID))
	rewritten.URL.Path = rest
	rewritten.URL.RawPath = ""
	h.streamable.ServeHTTP(w, rewritten)
}
