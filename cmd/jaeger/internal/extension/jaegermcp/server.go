// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
    "context"
    _ "embed"
    "errors"
    "fmt"
    "net"
    "net/http"
    "time"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "go.opentelemetry.io/collector/component"
    "go.opentelemetry.io/collector/extension"
    "go.opentelemetry.io/collector/extension/extensioncapabilities"
    "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
    "go.uber.org/zap"

    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/handlers"
    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/skills"
    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
    "github.com/jaegertracing/jaeger/internal/tenancy"
)

//go:embed INSTRUCTIONS.md
var serverInstructions string

var (
    _ extension.Extension             = (*server)(nil)
    _ extensioncapabilities.Dependent = (*server)(nil)
)

// server implements the Jaeger MCP extension.
type server struct {
    config       *Config
    telset       component.TelemetrySettings
    httpServer   *http.Server
    listener     net.Listener
    mcpServer    *mcp.Server
    queryAPI     *querysvc.QueryService
    skillsLoader *skills.Loader
}

// newServer creates a new MCP server instance.
func newServer(config *Config, telset component.TelemetrySettings) *server {
    return &server{
        config: config,
        telset: telset,
    }
}

// Dependencies implements extensioncapabilities.Dependent to ensure
// this extension starts after the jaegerquery extension.
func (*server) Dependencies() []component.ID {
    return []component.ID{jaegerquery.ID}
}

// registeredToolLister implements skills.ToolLister backed by a static list
// of tool names already registered on the MCP server.
type registeredToolLister struct {
    names []string
}

func (r *registeredToolLister) ListTools(_ context.Context) ([]string, error) {
    return r.names, nil
}

// Start initializes and starts the MCP server.
func (s *server) Start(ctx context.Context, host component.Host) error {
    s.telset.Logger.Info("Starting Jaeger MCP server", zap.String("endpoint", s.config.HTTP.NetAddr.Endpoint))

    // Get v2 QueryService from jaegerquery extension.
    queryExt, err := jaegerquery.GetExtension(host)
    if err != nil {
        return fmt.Errorf("cannot get %s extension: %w", jaegerquery.ID, err)
    }
    s.queryAPI = queryExt.QueryService()

    tenancyMgr := queryExt.TenancyManager()
    s.mcpServer = mcp.NewServer(
        &mcp.Implementation{
            Name:    s.config.ServerName,
            Version: s.config.ServerVersion,
        },
        &mcp.ServerOptions{
            Instructions: serverInstructions,
        },
    )
    s.registerTools()

    // Load skills after core tools are registered so the lister snapshot is accurate.
    s.skillsLoader = skills.NewLoader(s.config.SkillsDir, s.telset.Logger)
    lister := &registeredToolLister{names: registeredToolNames()}
    if err := s.skillsLoader.Load(ctx, lister); err != nil {
        // Log but do not block startup — invalid skill files are an operator error.
        s.telset.Logger.Warn("some skills failed to load", zap.Error(err))
    }
    s.registerSkillsTools()

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
            JSONResponse:   false,
            Stateless:      false,
            SessionTimeout: 5 * time.Minute,
        },
    )

    tenantHandler := tenancy.ExtractTenantHTTPHandler(tenancyMgr, mcpHandler)
    handler := otelhttp.NewHandler(
        tenantHandler,
        "jaeger_mcp",
        otelhttp.WithTracerProvider(s.telset.TracerProvider),
    )

    s.listener, err = s.config.HTTP.ToListener(ctx)
    if err != nil {
        return fmt.Errorf("failed to listen on %s: %w", s.config.HTTP.NetAddr.Endpoint, err)
    }

    s.httpServer, err = s.config.HTTP.ToServer(
        ctx,
        host.GetExtensions(),
        s.telset,
        handler,
    )
    if err != nil {
        s.listener.Close()
        return fmt.Errorf("failed to create HTTP server: %w", err)
    }

    go func() {
        if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
            s.telset.Logger.Error("MCP server error", zap.Error(err))
        }
    }()

    s.telset.Logger.Info("Jaeger MCP server started successfully",
        zap.String("mcp_endpoint", "http://"+s.config.HTTP.NetAddr.Endpoint+"/mcp"))
    return nil
}

// Shutdown gracefully stops the MCP server.
func (s *server) Shutdown(ctx context.Context) error {
    s.telset.Logger.Info("Shutting down Jaeger MCP server")
    if s.httpServer != nil {
        if err := s.httpServer.Shutdown(ctx); err != nil {
            return fmt.Errorf("failed to shutdown HTTP server: %w", err)
        }
    }
    return nil
}

// registeredToolNames returns the names of all core MCP tools registered by registerTools().
// This list is used to build the ToolLister snapshot for skill validation.
func registeredToolNames() []string {
    return []string{
        "health",
        "get_services",
        "get_span_names",
        "search_traces",
        "get_span_details",
        "get_trace_errors",
        "get_trace_topology",
        "get_critical_path",
        "get_service_dependencies",
    }
}

// registerTools registers all core MCP tools with the server.
func (s *server) registerTools() {
    mcp.AddTool(s.mcpServer, &mcp.Tool{
        Name:        "health",
        Description: "Check if the Jaeger MCP server is running. Returns server status, name, and version.",
    }, s.healthTool)

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
}

// registerSkillsTools registers the list_skills and get_skill MCP tools.
func (s *server) registerSkillsTools() {
    mcp.AddTool(s.mcpServer, &mcp.Tool{
        Name: "list_skills",
        Description: "List all available AI debugging skills. Each skill provides a specialized " +
            "analysis workflow (e.g. critical path analysis, N+1 query detection). " +
            "Use get_skill to retrieve the full definition of a skill.",
    }, handlers.NewListSkillsHandler(s.skillsLoader))

    mcp.AddTool(s.mcpServer, &mcp.Tool{
        Name: "get_skill",
        Description: "Retrieve the full definition of a named skill, including its system prompt, " +
            "allowed tools, and examples. Use this before invoking a skill to understand " +
            "its expected tool call sequence.",
    }, handlers.NewGetSkillHandler(s.skillsLoader))
}

// HealthToolOutput is the strongly-typed output for the health tool.
type HealthToolOutput struct {
    Status  string `json:"status" jsonschema:"Server status (ok/error)"`
    Server  string `json:"server" jsonschema:"Server name"`
    Version string `json:"version" jsonschema:"Server version"`
}

func (s *server) healthTool(
    _ context.Context,
    _ *mcp.CallToolRequest,
    _ struct{},
) (*mcp.CallToolResult, HealthToolOutput, error) {
    return nil, HealthToolOutput{
        Status:  "ok",
        Server:  s.config.ServerName,
        Version: s.config.ServerVersion,
    }, nil
}