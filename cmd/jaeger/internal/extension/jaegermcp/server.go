// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
	"go.yaml.in/yaml/v3"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/handlers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

//go:embed INSTRUCTIONS.md
var serverInstructions string

//go:embed all:skills
var skillsEmbedFS embed.FS

var (
	_ extension.Extension             = (*server)(nil)
	_ extensioncapabilities.Dependent = (*server)(nil)
)

// server implements the Jaeger MCP extension.
type server struct {
	config     *Config
	telset     component.TelemetrySettings
	httpServer *http.Server
	listener   net.Listener
	mcpServer  *mcp.Server
	queryAPI   *querysvc.QueryService
	bgFinished sync.WaitGroup
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

	s.bgFinished.Go(func() {
		if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.telset.Logger.Error("MCP server error", zap.Error(err))
		}
	})

	s.telset.Logger.Info("Jaeger MCP server started successfully",
		zap.String("mcp_endpoint", "http://"+s.config.HTTP.NetAddr.Endpoint+"/mcp"))
	return nil
}

// Shutdown gracefully stops the MCP server.
func (s *server) Shutdown(ctx context.Context) error {
	s.telset.Logger.Info("Shutting down Jaeger MCP server")

	var err error
	if s.httpServer != nil {
		err = s.httpServer.Shutdown(ctx)
	}
	s.bgFinished.Wait()
	if err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}
	return nil
}

// registerTools registers all MCP tools with the server.
func (s *server) registerTools() {
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

	sFS, _ := fs.Sub(skillsEmbedFS, "skills")
	gateway := buildSkillsGateway(sFS)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "read_skill",
		Description: "Read skills available for trace analysis. " +
			"Call with no path to get a catalog of all available skills with descriptions. " +
			"Call with '<skill-name>/SKILL.md' to load a skill's full procedure.",
	}, handlers.NewReadSkillHandler(sFS, s.config.MaxReadFileSize, gateway))
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func buildSkillsGateway(skillsFS fs.FS) string {
	entries, err := fs.ReadDir(skillsFS, ".")
	if err != nil {
		return "# Available Skills\n\nNo skills found.\n"
	}
	var b strings.Builder
	b.WriteString("# Available Skills\n")
	b.WriteString("Call read_skill(\"<skill-name>/SKILL.md\") to load a skill's full procedure before applying it.\n\n")
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(skillsFS, e.Name()+"/SKILL.md")
		if err != nil {
			continue
		}
		fm := extractFrontmatter(data)
		if fm.Name != "" && fm.Description != "" {
			fmt.Fprintf(&b, "- %s — %s\n", fm.Name, fm.Description)
		}
	}
	return b.String()
}

func extractFrontmatter(content []byte) skillFrontmatter {
	const fence = "---"
	s := string(content)
	if !strings.HasPrefix(s, fence) {
		return skillFrontmatter{}
	}
	rest := s[len(fence):]
	idx := strings.Index(rest, "\n"+fence)
	if idx < 0 {
		return skillFrontmatter{}
	}
	var fm skillFrontmatter
	dec := yaml.NewDecoder(bytes.NewReader([]byte(rest[:idx])))
	if err := dec.Decode(&fm); err != nil {
		return skillFrontmatter{}
	}
	return fm
}
