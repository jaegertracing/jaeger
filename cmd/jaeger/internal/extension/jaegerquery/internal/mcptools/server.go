// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package mcptools provides the Jaeger telemetry MCP tools as a reusable
// library. The tools wrap a *querysvc.QueryService. NewHandler returns a
// closeable Handler serving them over streamable HTTP, so any component holding a
// QueryService can expose the Jaeger telemetry tools over MCP without
// re-implementing the handlers; a component that needs the server to dispatch more
// than the built-in tools (e.g. the AI gateway's per-turn UI tools) layers its own
// receiving middleware on with Handler.AddReceivingMiddleware.
package mcptools

import (
	"embed"
	"errors"
	"io"
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

// newServer builds an *mcp.Server with the Jaeger telemetry tools and the
// tracing/metrics middleware registered. It takes *querysvc.QueryService
// directly (rather than fetching it from the component host), which keeps it
// dependency-free of the jaegerquery extension package and avoids the import
// cycle a host-based lookup would create now that the tools live under
// jaegerquery/internal.
func newServer(telset telemetry.Settings, queryAPI *querysvc.QueryService, cfg Config) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    cfg.ServerName,
			Version: cfg.ServerVersion,
		},
		&mcp.ServerOptions{
			Instructions: serverInstructions,
		},
	)
	registerTools(server, queryAPI, cfg)

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
	return server
}

// Handler serves an *mcp.Server over HTTP and owns that server's teardown: Close
// reaps the sessions still bound to it, so a mount gets its teardown from
// construction instead of from whoever remembers to reach for the server. Mounting
// one Handler at several paths — jaeger-query serves it at /api/ai/mcp/, and the AI
// gateway serves it again under the turn-scoped routes — gives those paths one
// server and one session map, so a session established through either resolves on
// both.
type Handler struct {
	http.Handler
	server *mcp.Server
}

var _ io.Closer = (*Handler)(nil)

// Close reaps every session still bound to the server. The SDK reaps a session
// only when it goes idle (see StreamableHTTPOptions.SessionTimeout), so without
// this a live session would outlive the server. Sessions() yields a snapshot (it
// clones under lock), so closing each one mid-iteration — which deregisters it —
// is safe.
func (h *Handler) Close() error {
	var errs []error
	for session := range h.server.Sessions() {
		errs = append(errs, session.Close())
	}
	return errors.Join(errs...)
}

// AddReceivingMiddleware layers middleware onto the wrapped server after it has
// been built, so a caller that did not construct the server can still extend what
// it dispatches — the AI gateway uses this to add its per-turn UI tools to the
// shared telemetry server. The HTTP shell captures the server by pointer, so
// middleware added here applies to every later request; call it during startup,
// before the HTTP server begins serving.
func (h *Handler) AddReceivingMiddleware(middleware ...mcp.Middleware) {
	h.server.AddReceivingMiddleware(middleware...)
}

// NewHandler builds a closeable Handler that serves the Jaeger telemetry MCP tools
// over streamable HTTP, backed by the given QueryService — the shared endpoint at
// jaeger-query's /api/ai/mcp/. One *mcp.Server is reused for every session: with
// Stateless: false the SDK builds one ServerSession per MCP session and reuses it
// for that session's requests. It binds no listener of its own — the caller mounts
// the returned handler on an existing mux and closes it at shutdown so its MCP
// sessions are reaped.
func NewHandler(telset telemetry.Settings, queryAPI *querysvc.QueryService, tenancyMgr *tenancy.Manager, cfg Config) *Handler {
	server := newServer(telset, queryAPI, cfg)
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			JSONResponse:   false, // Use SSE for streamed events
			Stateless:      false, // Session state management
			SessionTimeout: mcpSessionTimeout,
		},
	)
	tenantHandler := tenancy.ExtractTenantHTTPHandler(tenancyMgr, streamable)
	return &Handler{
		Handler: otelhttp.NewHandler(
			tenantHandler,
			"jaeger_mcp",
			otelhttp.WithTracerProvider(telset.TracerProvider),
		),
		server: server,
	}
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
	}, handlers.NewReadSkillHandler(s.skillsFS, s.config.CustomSkillsFS, s.config.MaxReadFileSize))
}
