// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/closers"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// RegisterParams is what Register needs from the query server: the ai block
// of its configuration and the few server facts the AI routes depend on.
type RegisterParams struct {
	Config *Config
	// BasePath is the query server's URL prefix, under which every AI route is
	// mounted.
	BasePath string
	// HTTPEndpoint and TLSEnabled describe the query server's own listener,
	// from which the MCP base URL announced to the sidecar is inferred when the
	// configuration does not set one.
	HTTPEndpoint string
	TLSEnabled   bool
	QueryService *querysvc.QueryService
	Tenancy      *tenancy.Manager
	Telemetry    telemetry.Settings
}

// Routes is what Register hands back to the query server: a closer for what
// it mounted, and the ai.auth decision for the one surface the query server
// renders itself, the aiAssistant flag in the UI page.
type Routes struct {
	closers closers.Closers
	auth    *authenticator
}

// Close releases everything Register mounted.
func (rs *Routes) Close() error {
	return rs.closers.Close()
}

// Admits reports whether the ai.auth authenticator accepts r. Without an
// ai.auth block every request is admitted.
func (rs *Routes) Admits(r *http.Request) bool {
	return rs.auth == nil || rs.auth.admits(r)
}

// Register mounts the AI surface of the query port on r: the telemetry MCP
// endpoint, the AI chat gateway, and the ai.auth check in front of both.
// Invalid AI config disables the AI surface rather than stopping the query
// server, which also serves the UI and the trace APIs. An ai.auth block that
// names an unusable extension is the exception and stops startup, since
// serving the AI surface unguarded is the one outcome an operator who set it
// cannot want.
func Register(ctx context.Context, r *http.ServeMux, p RegisterParams) (*Routes, error) {
	aiCfg := p.Config
	telset := p.Telemetry
	if err := aiCfg.Validate(); err != nil {
		telset.Logger.Error("Invalid AI config, AI handler disabled", zap.Error(err))
		return &Routes{}, nil
	}

	// One authenticator guards every AI surface, so a caller admitted to the chat
	// panel is admitted to the MCP tools and the other way round.
	var auth *authenticator
	if authCfg := aiCfg.Auth.Get(); authCfg != nil {
		var err error
		auth, err = newAuthenticator(ctx, authCfg, telset.Host)
		if err != nil {
			return nil, err
		}
	}

	var cs closers.Closers
	// The telemetry MCP endpoint stands on its own: external MCP clients (Claude Code,
	// Cursor, IDEs) dial it with no chat sidecar involved, so it is built and mounted
	// whenever ai.mcp is configured, never depending on ai.agent_url. It comes before
	// the gateway because the gateway layers its per-turn UI tools onto this same
	// *mcp.Server rather than standing up a second one.
	var mcpHandler *mcptools.Handler
	if mcp := aiCfg.MCP.Get(); mcp != nil {
		mcpParams := mcptools.DefaultServerParams()
		// Opened once, so a broken path is reported once, at startup. It stays open
		// for as long as it serves, so it is released with the server rather than at
		// process exit.
		customSkills, err := mcptools.OpenCustomSkillsDir(mcp.SkillsDir)
		if err != nil {
			return nil, err
		}
		if customSkills != nil {
			cs = append(cs, customSkills)
		}
		mcpParams.CustomSkillsFS = customSkills
		mcpParams.StreamableHTTP = mcp.StreamableHTTP

		mcpHandler = mcptools.NewHandler(telset, p.QueryService, p.Tenancy, mcpParams)
		cs = append(cs, mcpHandler)
		// The shared endpoint and the gateway's turn-scoped /api/ai/mcp/<id>/ patterns
		// coexist because the latter are strictly more specific for ServeMux.
		prefix := strings.TrimSuffix(p.BasePath, "/") + "/api/ai/mcp"
		var shared http.Handler = mcpHandler
		if auth != nil {
			shared = auth.wrap(shared)
		}
		r.Handle(prefix+"/", http.StripPrefix(prefix, shared))
		telset.Logger.Info("Jaeger telemetry MCP endpoint enabled", zap.String("path", prefix+"/"))
	}

	if aiCfg.AgentURL != "" {
		var chatMiddleware func(http.Handler) http.Handler
		if auth != nil {
			chatMiddleware = auth.wrap
		}
		// jaegerai owns the chat endpoint and, when MCP is configured, the turn-scoped
		// endpoint (/api/ai/mcp/<id>/) — which it serves off mcpHandler's server, with
		// the turn's UI tools layered on. Passing nil leaves it chat-only: there is no
		// point advertising UI tools with no telemetry tools behind them.
		//
		// The announced base URL is resolved here because inferring the
		// gateway's own localhost address needs the query HTTP endpoint and TLS
		// setting, which the query server passes in RegisterParams. This is the only thing
		// agent_url decides about MCP — which URL the sidecar is handed.
		aiGateway := NewHandler(HandlerParams{
			Logger:             telset.Logger,
			AgentURL:           aiCfg.AgentURL,
			AgentHeaders:       aiCfg.AgentHeaders,
			BasePath:           p.BasePath,
			MaxRequestBodySize: aiCfg.MaxRequestBodySize,
			MCP:                mcpHandler,
			MCPBaseURL:         aiCfg.ResolveMCPBaseURL(ctx, p.HTTPEndpoint, p.TLSEnabled),
			ChatMiddleware:     chatMiddleware,
		})
		aiGateway.RegisterRoutes(r)
	}
	return &Routes{closers: cs, auth: auth}, nil
}
