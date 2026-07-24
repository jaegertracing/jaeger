// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package mcptools provides the Jaeger telemetry MCP tools as a reusable
// library. The tools wrap a *querysvc.QueryService. NewServer returns an
// *mcp.Server with the tools registered; WrapHTTP serves a given *mcp.Server as
// an http.Handler over streamable HTTP; and NewHandler composes the two for the
// common session-free case — so any component holding a QueryService can expose
// the Jaeger telemetry tools over MCP (optionally layering its own tools or
// receiving middleware on the server first) without re-implementing the
// handlers.
package mcptools

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

//go:embed INSTRUCTIONS.md
var serverInstructions string

// skillsEmbedFS holds the built-in skill playbooks served by the read_skill
// tool. The root SKILL.md is a progressive-disclosure gateway to the sub-skills
// under skills/.
//
//go:embed all:skills
var skillsEmbedFS embed.FS

// NewServer builds an *mcp.Server with the Jaeger telemetry tools and the
// tracing/metrics middleware registered. It takes *querysvc.QueryService
// directly (rather than fetching it from the component host), which keeps it
// dependency-free of the jaegerquery extension package and avoids the import
// cycle a host-based lookup would create now that the tools live under
// jaegerquery/internal.
//
// Callers that only need the session-free telemetry endpoint should use
// NewHandler. Callers that need to layer additional behaviour on top (e.g. a
// session-scoped endpoint that advertises per-session UI tools via receiving
// middleware) build a server here, add their middleware, and serve it with
// WrapHTTP.
func NewServer(telset telemetry.Settings, queryAPI *querysvc.QueryService, cfg Config) (*mcp.Server, error) {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    cfg.ServerName,
			Version: cfg.ServerVersion,
		},
		&mcp.ServerOptions{
			Instructions: serverInstructions,
		},
	)
	if _, err := registerTools(server, queryAPI, cfg, telset.Logger); err != nil {
		return nil, err
	}

	mw := []mcp.Middleware{
		createTracingMiddleware(telset.TracerProvider),
	}
	metricsMiddleware, err := createMetricsMiddleware(telset.MeterProvider)
	if err != nil {
		telset.Logger.Warn("failed to create MCP metrics middleware, continuing without metrics", zap.Error(err))
	} else {
		mw = append(mw, metricsMiddleware)
	}
	server.AddReceivingMiddleware(mw...)
	return server, nil
}

// WrapHTTP serves an *mcp.Server as an http.Handler over streamable HTTP, with
// tenancy extraction and OTel HTTP instrumentation. It is the transport shell
// shared by the session-free endpoint and the session-scoped endpoint (which
// layers per-session UI tools onto the server via receiving middleware before
// wrapping it). The same server instance is reused for every session — with
// Stateless: false the SDK builds one ServerSession per MCP session and reuses
// it for that session's requests. It binds no listener of its own — the caller
// mounts the returned handler on an existing mux.
func WrapHTTP(server *mcp.Server, tenancyMgr *tenancy.Manager, telset telemetry.Settings) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			JSONResponse:   false, // Use SSE for streamed events
			Stateless:      false, // Session state management
			SessionTimeout: mcpSessionTimeout,
		},
	)
	tenantHandler := tenancy.ExtractTenantHTTPHandler(tenancyMgr, mcpHandler)
	return otelhttp.NewHandler(
		tenantHandler,
		"jaeger_mcp",
		otelhttp.WithTracerProvider(telset.TracerProvider),
	)
}

// NewHandler builds an http.Handler that serves the Jaeger telemetry MCP tools
// over streamable HTTP, backed by the given QueryService — the session-free
// endpoint (e.g. jaeger-query at /api/ai/mcp/). It is a thin composition of
// NewServer and WrapHTTP around a single shared server.
func NewHandler(telset telemetry.Settings, queryAPI *querysvc.QueryService, tenancyMgr *tenancy.Manager, cfg Config) (http.Handler, error) {
	server, err := NewServer(telset, queryAPI, cfg)
	if err != nil {
		return nil, err
	}
	return WrapHTTP(server, tenancyMgr, telset), nil
}

// addTool registers a tool on the server and records its name in names, so
// registration and the allowed-tools validation of operator skills share one
// source of truth (see registerTools).
func addTool[In, Out any](server *mcp.Server, names map[string]bool, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	names[tool.Name] = true
	mcp.AddTool(server, tool, handler)
}

// registerTools registers all Jaeger telemetry MCP tools on the given server,
// wiring each handler to the supplied QueryService. cfg supplies the per-tool
// limits (search results, span details, read-file size) and the optional
// operator skills directory; an unusable cfg.SkillsDir path is the only error
// case (see buildMergedSkillsFS). The returned set of tool names is the same
// one used to validate operator skills' allowed-tools lists, exposed so
// tests can assert it never drifts from the live ListTools() output.
func registerTools(server *mcp.Server, queryAPI *querysvc.QueryService, cfg Config, logger *zap.Logger) (map[string]bool, error) {
	s := struct { // alias to minimize code diff during move
		mcpServer *mcp.Server
		config    Config
		queryAPI  *querysvc.QueryService
		skillsFS  fs.FS
	}{
		mcpServer: server,
		config:    cfg,
		queryAPI:  queryAPI,
	}
	registeredToolNames := make(map[string]bool)
	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name:        "get_services",
		Description: "List service names known to Jaeger. Supports optional regex filtering via 'pattern'.",
	}, handlers.NewGetServicesHandler(s.queryAPI))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "get_span_names",
		Description: "List span/operation names for a given service, with their span kinds " +
			"(SERVER, CLIENT, INTERNAL, etc.)",
	}, handlers.NewGetSpanNamesHandler(s.queryAPI))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "search_traces",
		Description: "Search for traces matching filters. Returns lightweight summaries " +
			"(trace_id, duration, span_count, error flag, etc.) without individual spans or attributes.",
	}, handlers.NewSearchTracesHandler(s.queryAPI, s.config.MaxSearchResults))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "get_span_details",
		Description: "Fetch full span data (attributes, events, links, status) for specific spans. " +
			"Returns verbose output per span.",
	}, handlers.NewGetSpanDetailsHandler(s.queryAPI, s.config.MaxSpanDetailsPerRequest))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "get_trace_errors",
		Description: "Get full details for all error-status spans in a trace. " +
			"Results may be truncated to the server limit; " +
			"compare total_error_count with the number of returned spans to detect truncation.",
	}, handlers.NewGetTraceErrorsHandler(s.queryAPI, s.config.MaxSpanDetailsPerRequest))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "get_trace_topology",
		Description: "Get the structural overview of a trace as a flat, depth-first span list. " +
			"Each span includes a 'path' field encoding ancestry as slash-delimited span IDs " +
			"(e.g. 'rootID/parentID/spanID'). " +
			"Does NOT include attributes, events, or links.",
	}, handlers.NewGetTraceTopologyHandler(s.queryAPI, s.config.MaxSpanDetailsPerRequest))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "get_critical_path",
		Description: "Identify the critical latency path through a trace: the chain of spans " +
			"that determined end-to-end duration. " +
			"Higher self_time_us values indicate where time is concentrated on the critical path.",
	}, handlers.NewGetCriticalPathHandler(s.queryAPI))

	addTool(s.mcpServer, registeredToolNames, &mcp.Tool{
		Name: "get_service_dependencies",
		Description: "Get the service dependency graph showing caller-callee pairs. " +
			"Returns edges with call counts over a configurable time window (default: last 24h).",
	}, handlers.NewGetDependenciesHandler(s.queryAPI))

	readSkillTool := &mcp.Tool{
		Name: "read_skill",
		Description: "Read a skill file for trace analysis. " +
			"Skills are organized using progressive disclosure. " +
			"Start with SKILL.md which will guide you to more specific sub-skills. " +
			"The root SKILL.md also points to custom/SKILL.md, an operator-supplied " +
			"catalog of installation-specific skills (served under custom/ when the " +
			"operator has configured one) — always check it too, since it may contain " +
			"skills tailored to this Jaeger installation that the built-in catalog " +
			"does not know about.",
	}
	// read_skill is registered below (after the FS it serves is built), but
	// it must already count as a registered tool when operator skills'
	// allowed-tools lists are validated.
	registeredToolNames[readSkillTool.Name] = true

	// error not possible because we embed /skills dir
	builtins, _ := fs.Sub(skillsEmbedFS, "skills")
	var err error
	s.skillsFS, err = buildMergedSkillsFS(builtins, s.config.SkillsDir, registeredToolNames, logger)
	if err != nil {
		return nil, err
	}
	addTool(s.mcpServer, registeredToolNames, readSkillTool,
		handlers.NewReadSkillHandler(s.skillsFS, s.config.MaxReadFileSize))
	return registeredToolNames, nil
}
