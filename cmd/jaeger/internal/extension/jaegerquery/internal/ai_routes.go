// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// aiRoutes is what registerAIRoutes hands back to the router: the closers for
// what it mounted, and the capability provider after the ai.auth gate.
type aiRoutes struct {
	closers      closers
	capabilities BackendCapabilityProvider
}

func (a *aiRoutes) Close() error {
	return a.closers.Close()
}

// registerAIRoutes mounts the AI surface of the query port: the telemetry MCP
// endpoint, the chat gateway, and the ai.auth check in front of both. It
// returns what the rest of the router needs and nothing more: a closer for
// what it mounted, and the capability provider with the AI panel hidden from
// callers the authenticator rejects. Invalid AI config disables the AI surface
// rather than stopping the query server, which also serves the UI and the
// trace APIs. An ai.auth block that names an unusable extension is the
// exception and stops startup, since serving the AI surface unguarded is the
// one outcome an operator who set it cannot want.
func registerAIRoutes(
	ctx context.Context,
	r *http.ServeMux,
	queryOpts *QueryOptions,
	querySvc *querysvc.QueryService,
	tenancyMgr *tenancy.Manager,
	telset telemetry.Settings,
	backendCaps BackendCapabilityProvider,
) (*aiRoutes, error) {
	aiCfg := queryOpts.AI.Get()
	if err := aiCfg.Validate(); err != nil {
		telset.Logger.Error("Invalid AI config, AI handler disabled", zap.Error(err))
		return &aiRoutes{capabilities: backendCaps}, nil
	}

	// One authenticator guards every AI surface, so a caller admitted to the chat
	// panel is admitted to the MCP tools and the other way round.
	authenticator, err := newAIAuthenticator(ctx, aiCfg.Auth.Get(), telset.Host)
	if err != nil {
		return nil, err
	}

	var cs closers
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

		mcpHandler = mcptools.NewHandler(telset, querySvc, tenancyMgr, mcpParams)
		cs = append(cs, mcpHandler)
		mountSharedMCP(r, authenticator.Wrap(mcpHandler), queryOpts.BasePath, telset)
	}

	if aiCfg.AgentURL != "" {
		// jaegerai owns the chat endpoint and, when MCP is configured, the turn-scoped
		// endpoint (/api/ai/mcp/<id>/) — which it serves off mcpHandler's server, with
		// the turn's UI tools layered on. Passing nil leaves it chat-only: there is no
		// point advertising UI tools with no telemetry tools behind them.
		//
		// The announced base URL is resolved here because inferring the
		// gateway's own localhost address needs the query HTTP endpoint and TLS
		// setting, which live on QueryOptions, not AIConfig. This is the only thing
		// agent_url decides about MCP — which URL the sidecar is handed.
		aiGateway := jaegerai.NewHandler(jaegerai.HandlerParams{
			Logger:             telset.Logger,
			AgentURL:           aiCfg.AgentURL,
			AgentHeaders:       aiCfg.AgentHeaders,
			BasePath:           queryOpts.BasePath,
			MaxRequestBodySize: aiCfg.MaxRequestBodySize,
			MCP:                mcpHandler,
			MCPBaseURL:         aiCfg.resolveMCPBaseURL(ctx, queryOpts.HTTP.NetAddr.Endpoint, queryOpts.HTTP.TLS.HasValue()),
			ChatMiddleware:     authenticator.Wrap,
		})
		aiGateway.RegisterRoutes(r)
	}
	return &aiRoutes{closers: cs, capabilities: authenticator.GateCapabilities(backendCaps)}, nil
}

// mountSharedMCP mounts handler as the shared telemetry MCP endpoint at
// <basePath>/api/ai/mcp/, stripping that prefix so the handler sees its own root.
// This pattern coexists with the AI gateway's turn-scoped ones because those are
// strictly more specific, so ServeMux sends /api/ai/mcp/<id>/ to the gateway and
// everything else here.
func mountSharedMCP(r *http.ServeMux, handler http.Handler, basePath string, telset telemetry.Settings) {
	prefix := strings.TrimSuffix(basePath, "/") + "/api/ai/mcp"
	r.Handle(prefix+"/", http.StripPrefix(prefix, handler))
	telset.Logger.Info("Jaeger telemetry MCP endpoint enabled", zap.String("path", prefix+"/"))
}

// per-route wrap is the only instrumentation layer.
