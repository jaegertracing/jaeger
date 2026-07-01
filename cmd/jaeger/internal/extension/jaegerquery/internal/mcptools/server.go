// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package mcptools provides the Jaeger telemetry MCP tools as a reusable
// library. The tools wrap a *querysvc.QueryService; RegisterTools mounts them
// on an *mcp.Server, and NewHandler builds an http.Handler serving them over
// streamable HTTP — so any component holding a QueryService can expose the
// Jaeger telemetry tools over MCP without re-implementing the handlers.
package mcptools

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
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

// NewHandler builds an http.Handler that serves the Jaeger telemetry MCP tools
// over streamable HTTP, backed directly by the given QueryService. It carries
// the same tracing/metrics middleware and tenancy extraction the standalone
// jaeger_mcp extension used, but binds no listener of its own — the caller
// mounts it on an existing mux (e.g. jaeger-query at /api/ai/mcp/).
//
// Taking *querysvc.QueryService directly (rather than fetching it from the
// component host) is what keeps this dependency-free of the jaegerquery
// extension package, avoiding the import cycle a host-based lookup would create
// now that the tools live under jaegerquery/internal.
func NewHandler(telset component.TelemetrySettings, queryAPI *querysvc.QueryService, tenancyMgr *tenancy.Manager, cfg Config) http.Handler {
	s := struct { // alias to minimize code diff during move
		config    Config
		telset    component.TelemetrySettings
		mcpServer *mcp.Server
	}{
		config: cfg,
		telset: telset,
	}
	s.mcpServer = mcp.NewServer(
		&mcp.Implementation{
			Name:    s.config.ServerName,
			Version: s.config.ServerVersion,
		},
		&mcp.ServerOptions{
			Instructions: serverInstructions,
		},
	)
	registerTools(s.mcpServer, queryAPI, cfg)

	mw := []mcp.Middleware{
		createTracingMiddleware(s.telset.TracerProvider),
	}
	metricsMiddleware, err := createMetricsMiddleware(s.telset.MeterProvider)
	if err != nil {
		s.telset.Logger.Warn("failed to create MCP metrics middleware, continuing without metrics", zap.Error(err))
	} else {
		mw = append(mw, metricsMiddleware)
	}
	s.mcpServer.AddReceivingMiddleware(mw...)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return s.mcpServer },
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
		otelhttp.WithTracerProvider(s.telset.TracerProvider),
	)
}

// RegisterTools registers all Jaeger telemetry MCP tools on the given server,
// wiring each handler to the supplied QueryService. cfg supplies the per-tool
// limits (search results, span details, read-file size).
func registerTools(server *mcp.Server, queryAPI *querysvc.QueryService, cfg Config) {
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
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_services",
		Description: "List service names known to Jaeger. Supports optional regex filtering via 'pattern'.",
	}, handlers.NewGetServicesHandler(s.queryAPI))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_span_names",
		Description: "List span/operation names for a given service, with their span kinds " +
			"(SERVER, CLIENT, INTERNAL, etc.)",
	}, handlers.NewGetSpanNamesHandler(s.queryAPI))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "search_traces",
		Description: "Search for traces matching filters. Returns lightweight summaries " +
			"(trace_id, duration, span_count, error flag, etc.) without individual spans or attributes.",
	}, handlers.NewSearchTracesHandler(s.queryAPI, s.config.MaxSearchResults))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_span_details",
		Description: "Fetch full span data (attributes, events, links, status) for specific spans. " +
			"Returns verbose output per span.",
	}, handlers.NewGetSpanDetailsHandler(s.queryAPI, s.config.MaxSpanDetailsPerRequest))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_trace_errors",
		Description: "Get full details for all error-status spans in a trace. " +
			"Results may be truncated to the server limit; " +
			"compare total_error_count with the number of returned spans to detect truncation.",
	}, handlers.NewGetTraceErrorsHandler(s.queryAPI, s.config.MaxSpanDetailsPerRequest))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_trace_topology",
		Description: "Get the structural overview of a trace as a flat, depth-first span list. " +
			"Each span includes a 'path' field encoding ancestry as slash-delimited span IDs " +
			"(e.g. 'rootID/parentID/spanID'). " +
			"Does NOT include attributes, events, or links.",
	}, handlers.NewGetTraceTopologyHandler(s.queryAPI, s.config.MaxSpanDetailsPerRequest))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_critical_path",
		Description: "Identify the critical latency path through a trace: the chain of spans " +
			"that determined end-to-end duration. " +
			"Higher self_time_us values indicate where time is concentrated on the critical path.",
	}, handlers.NewGetCriticalPathHandler(s.queryAPI))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_service_dependencies",
		Description: "Get the service dependency graph showing caller-callee pairs. " +
			"Returns edges with call counts over a configurable time window (default: last 24h).",
	}, handlers.NewGetDependenciesHandler(s.queryAPI))

	// error not possible because we embed /skills dir
	s.skillsFS, _ = fs.Sub(skillsEmbedFS, "skills")
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "read_skill",
		Description: "Read a skill file for trace analysis. " +
			"Skills are organized using progressive disclosure. " +
			"Start with SKILL.md which will guide you to more specific sub-skills.",
	}, handlers.NewReadSkillHandler(s.skillsFS, s.config.MaxReadFileSize))
}
