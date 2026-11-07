// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// Handler serves an *mcp.Server over HTTP. jaeger-query serves the Handler itself
// at /api/ai/mcp/; the AI gateway serves the same Handler under its turn-scoped
// routes, so both mounts share one server and its tools are registered once.
type Handler struct {
	// http is the chain a request runs through: the jaeger_mcp server span, then
	// tenant extraction, then the SDK's streamable HTTP handler.
	http   http.Handler
	server *mcp.Server
	// recovery guards the middleware added through AddReceivingMiddleware, which
	// the SDK places outside the recovery newServer installs.
	recovery mcp.Middleware
}

// NewHandler serves the Jaeger telemetry MCP tools over streamable HTTP, backed by
// queryAPI. It speaks MCP revision 2026-07-28, which has no sessions: each request
// is served from its own context and nothing outlives it, so any replica can serve
// any request, and GET and DELETE are answered 405. The caller mounts the handler
// on an existing mux.
func NewHandler(telset telemetry.Settings, queryAPI *querysvc.QueryService, tenancyMgr *tenancy.Manager, params ServerParams) *Handler {
	h := &Handler{
		server:   newServer(telset, queryAPI, params),
		recovery: createRecoveryMiddleware(telset.Logger),
	}
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return h.server },
		&mcp.StreamableHTTPOptions{
			JSONResponse:               params.StreamableHTTP.JSONResponse,
			Stateless:                  true,
			DisableLocalhostProtection: params.StreamableHTTP.DisableLocalhostProtection,
		},
	)
	h.http = otelhttp.NewHandler(
		tenancy.ExtractTenantHTTPHandler(tenancyMgr, streamable),
		"jaeger_mcp",
		otelhttp.WithTracerProvider(telset.TracerProvider),
	)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.http.ServeHTTP(w, r)
}

// AddReceivingMiddleware layers middleware onto the wrapped server after it has
// been built, so a caller that did not construct the server can still extend what
// it dispatches: the AI gateway uses this to add its per-turn UI tools to the
// shared telemetry server. Call it during startup, before the HTTP server begins
// serving; middleware added here applies to every later request.
//
// The SDK wraps the existing chain with each later call, so the given middleware
// runs outside the tracing, metrics and recovery middleware newServer installs,
// and a call it answers itself (as the gateway does for its UI tools) never
// reaches them. A recovery middleware is therefore added outside it, so a panic
// in the caller's middleware fails only that request.
func (h *Handler) AddReceivingMiddleware(middleware ...mcp.Middleware) {
	h.server.AddReceivingMiddleware(append([]mcp.Middleware{h.recovery}, middleware...)...)
}
