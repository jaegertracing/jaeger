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
	basePath string
	// chat is the chat endpoint (/api/ai/chat), always present.
	chat *chatEndpoint
	// mcp is the turn-scoped MCP endpoint. Non-nil only when the operator enabled
	// MCP (HandlerParams.EnableMCP); otherwise the endpoint is not mounted and the
	// gateway advertises AI chat only.
	mcp *turnScopedEndpoint
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
	// SkillsDir is the operator's skills directory (ai.skills_dir), threaded
	// into the turn-scoped MCP endpoint so it serves the same merged skill
	// tree as the shared one. Empty means built-in skills only.
	SkillsDir string
}

// NewHandler constructs a jaegerai.Handler, building the endpoints it will mount.
// basePath is normalized once so the registered mux patterns use a single
// canonical prefix. The chat and turn-scoped MCP endpoints share one turnRegistry
// so a chat turn and its MCP callbacks resolve to the same turn. When p.EnableMCP
// is set, the turn-scoped MCP endpoint is built from the supplied query service,
// tenancy manager, and telemetry settings; an unusable p.SkillsDir path fails
// construction (broken configuration).
func NewHandler(p HandlerParams) (*Handler, error) {
	basePath := normalizeBasePath(p.BasePath)
	turns := newTurnRegistry()
	h := &Handler{
		basePath: basePath,
		chat:     newChatEndpoint(p.Logger, NewContextualToolsStore(), turns, p.AgentURL, basePath, p.MaxRequestBodySize),
	}
	if p.EnableMCP {
		mcp, err := newTurnScopedEndpoint(p.Telset, p.QueryService, p.TenancyMgr, turns, basePath, p.SkillsDir, p.Logger)
		if err != nil {
			return nil, err
		}
		h.mcp = mcp
	}
	return h, nil
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
	router.HandleFunc(h.basePath+routeChat, h.chat.ServeHTTP)
	if h.mcp != nil {
		h.mcp.registerRoutes(router)
	}
}
