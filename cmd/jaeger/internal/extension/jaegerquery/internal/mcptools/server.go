// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package mcptools serves the Jaeger telemetry tools over MCP. The tools query a
// *querysvc.QueryService; NewHandler serves them over streamable HTTP, and
// Handler.AddReceivingMiddleware lets the AI gateway add its per-turn UI tools to
// the same server.
package mcptools

import (
	"embed"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
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
// tracing/metrics/recovery middleware registered. It takes *querysvc.QueryService
// directly (rather than fetching it from the component host), which keeps it
// dependency-free of the jaegerquery extension package and avoids the import
// cycle a host-based lookup would create now that the tools live under
// jaegerquery/internal.
func newServer(telset telemetry.Settings, queryAPI *querysvc.QueryService, params ServerParams) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    params.ServerName,
			Version: params.ServerVersion,
		},
		&mcp.ServerOptions{
			Instructions: serverInstructions,
		},
	)
	registerTools(server, queryAPI, params)

	mw := []mcp.Middleware{
		createTracingMiddleware(telset.TracerProvider),
	}
	metricsMiddleware, err := createMetricsMiddleware(telset.MeterProvider)
	if err != nil {
		telset.Logger.Warn("failed to create MCP metrics middleware, continuing without metrics", zap.Error(err))
	} else {
		mw = append(mw, metricsMiddleware)
	}
	// Innermost, so tracing and metrics record a recovered panic as a failed call.
	// Middleware added later through Handler.AddReceivingMiddleware wraps this
	// chain, so the Handler guards it with a recovery of its own.
	mw = append(mw, createRecoveryMiddleware(telset.Logger))
	server.AddReceivingMiddleware(mw...)
	return server
}

// RegisterTools registers all Jaeger telemetry MCP tools on the given server,
// wiring each handler to the supplied QueryService. params supplies the per-tool
// limits (search results, span details, read-file size).
func registerTools(server *mcp.Server, queryAPI *querysvc.QueryService, params ServerParams) {
	s := struct { // alias to minimize code diff during move
		mcpServer *mcp.Server
		params    ServerParams
		queryAPI  *querysvc.QueryService
		skillsFS  fs.FS
	}{
		mcpServer: server,
		params:    params,
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
		Name: "find_traces",
		Description: "Find traces matching one SQL statement in 'sql'. Returns lightweight summaries " +
			"(trace_id, duration, span_count, error flag, etc.) without individual spans or attributes. " +
			"The statement is SELECT DISTINCT trace_id FROM spans WHERE ... or SELECT * FROM traces " +
			"WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...), with both bounds on start_time.",
	}, handlers.NewFindTracesHandler(s.queryAPI, s.params.MaxSearchResults))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_span_details",
		Description: "Fetch full span data (attributes, events, links, status) for specific spans. " +
			"Returns verbose output per span.",
	}, handlers.NewGetSpanDetailsHandler(s.queryAPI, s.params.MaxSpanDetailsPerRequest))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "find_spans",
		Description: "Find spans matching one SQL statement in 'sql', across however many traces. " +
			"Returns lightweight summaries (trace_id, span_id, service, span_name, duration, " +
			"status) without attributes or events; use get_span_details for those. " +
			"The statement is SELECT * FROM spans WHERE ... with both bounds on start_time.",
	}, handlers.NewFindSpansHandler(s.queryAPI, s.params.MaxSearchResults))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_trace_errors",
		Description: "Get full details for all error-status spans in a trace. " +
			"Results may be truncated to the server limit; " +
			"compare total_error_count with the number of returned spans to detect truncation.",
	}, handlers.NewGetTraceErrorsHandler(s.queryAPI, s.params.MaxSpanDetailsPerRequest))

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_trace_topology",
		Description: "Get the structural overview of a trace as a flat, depth-first span list. " +
			"Each span includes a 'path' field encoding ancestry as slash-delimited span IDs " +
			"(e.g. 'rootID/parentID/spanID'). " +
			"Results may be truncated to the server limit, keeping a depth-first prefix " +
			"(later roots and sibling subtrees are omitted); " +
			"compare total_span_count with the number of returned spans to detect truncation. " +
			"Does NOT include attributes, events, or links.",
	}, handlers.NewGetTraceTopologyHandler(s.queryAPI, s.params.MaxSpanDetailsPerRequest))

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
	}, handlers.NewReadSkillHandler(s.skillsFS, s.params.CustomSkillsFS, s.params.MaxReadFileSize))
}
