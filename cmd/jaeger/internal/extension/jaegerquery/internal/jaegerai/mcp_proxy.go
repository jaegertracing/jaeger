// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// routeMCPPrefix is the URL prefix the MCP proxy mounts at. Everything
// under it is dispatched to the wrapped streamable HTTP handler — but
// only after we peel off the AG-UI session id from the next path
// segment, which is what scopes the per-session UI tool list.
const routeMCPPrefix = "/api/ai/mcp/"

const (
	mcpServerName    = "jaeger-ai-gateway"
	mcpServerVersion = "0.1.0"
	// mcpSessionTimeout caps an idle MCP session. The streamable handler
	// keeps per-MCP-session state for SSE resumption and stream id
	// correlation. 5 minutes matches what jaeger_mcp uses for the same
	// knob.
	mcpSessionTimeout = 5 * time.Minute

	// upstreamMCPURL points at the in-process jaegermcp extension's HTTP
	// MCP endpoint. The MCP proxy dials this once at startup and forwards
	// every non-UI tool call back to it, which keeps every tool dispatch
	// inside the gateway's tracing/auth path.
	//
	// TODO: replace with autodiscovery via the OTel collector host
	// (jaegermcp.GetExtension(host).Endpoint()) plus an optional
	// ai.mcp_upstream_url config override. Hardcoding here keeps the
	// initial MCP-proxy change small; the swap is mechanical once the
	// jaegermcp public interface lands.
	upstreamMCPURL = "http://127.0.0.1:16687/mcp"

	// upstreamDialTimeout bounds the initial connect + initialize round
	// trip to the upstream MCP server. Short enough that a misconfigured
	// upstream URL doesn't block process startup; long enough that a
	// cold-started jaegermcp on the same host has time to bind.
	upstreamDialTimeout = 5 * time.Second
)

// MCPProxy is the gateway-side MCP server every ACP agent dials as its
// single MCP egress. Mounted at /api/ai/mcp/<sessionId>/* so the
// per-session UI tools (declared on the AG-UI chat request that opened
// the session) are scoped to the right browser stream and the right
// tool list.
//
// It advertises two kinds of tools on the same MCP server:
//
//   - UI tools the frontend declared for the session, dispatched to the
//     browser via the SSE stream the chat handler keeps open.
//   - Telemetry tools provided by the jaegermcp extension, forwarded
//     to the upstream MCP server over an in-process HTTP client and
//     returned to the agent verbatim.
//
// The endpoint speaks standard MCP so every off-the-shelf agent
// (claude-agent-acp, harukitosa/claude-code-acp, etc.) can use it as
// its single MCP server without protocol-specific glue. Routing every
// tool call through the gateway preserves inversion of control:
// observability, auth, and unified UI-vs-telemetry dispatch all live
// in one place rather than being duplicated across sidecars.
type MCPProxy struct {
	logger   *zap.Logger
	ctxTools *ContextualToolsStore
	streams  *SessionStreams
	handler  *mcp.StreamableHTTPHandler

	// upstreamURL is the HTTP MCP endpoint we dial for telemetry tools.
	// Set from upstreamMCPURL by NewMCPProxy; overridable via the
	// package-private newMCPProxyWithUpstream so tests can point at a
	// mock server.
	upstreamURL string
	// upstream is the long-lived MCP client session pointed at the
	// in-process jaegermcp HTTP endpoint. nil when the initial dial
	// failed — the proxy then advertises UI tools only and logs the
	// reason at construction time.
	upstream *mcp.ClientSession
	// upstreamTools is the cached tools/list result from the upstream
	// server, captured once at startup. Empty when upstream is nil.
	upstreamTools []*mcp.Tool

	// acpConnections tracks live MCP-over-ACP callbacks the agent has
	// opened via mcp/connect. Keyed by connectionId; populated when
	// HandleConnect succeeds and drained by HandleDisconnect. See
	// mcp_acp_dispatch.go for the lifecycle.
	acpConnections *mcpACPConnections
}

// NewMCPProxy wires the streamable HTTP handler, dials the upstream MCP
// server, caches its tool list, and gives the handler a getServer
// callback that builds a fresh, session-scoped *mcp.Server for each
// session id. The handler caches per-MCP-session, so getServer is hot
// on the very first call from a new agent connection and rare
// afterwards; constructing a Server is cheap so we don't bother
// memoising further.
//
// An upstream dial failure is logged and does NOT abort construction —
// the proxy stays usable for UI-tool dispatch even when the telemetry
// MCP server is unavailable. That keeps "jaegermcp disabled" and
// "jaegermcp slow to start" as gracefully-degraded states rather than
// startup blockers.
func NewMCPProxy(ctx context.Context, logger *zap.Logger, ctxTools *ContextualToolsStore, streams *SessionStreams) *MCPProxy {
	return newMCPProxyWithUpstream(ctx, logger, ctxTools, streams, upstreamMCPURL)
}

// newMCPProxyWithUpstream is the test-friendly constructor: it accepts
// an explicit upstream MCP URL instead of using the hardcoded default.
// Same behaviour otherwise — dials the upstream and primes the tool
// cache, degrading gracefully on dial failure.
func newMCPProxyWithUpstream(ctx context.Context, logger *zap.Logger, ctxTools *ContextualToolsStore, streams *SessionStreams, upstreamURL string) *MCPProxy {
	if logger == nil {
		logger = zap.NewNop()
	}
	p := &MCPProxy{
		logger:         logger,
		ctxTools:       ctxTools,
		streams:        streams,
		upstreamURL:    upstreamURL,
		acpConnections: newMCPACPConnections(),
	}
	p.handler = mcp.NewStreamableHTTPHandler(p.serverForRequest, &mcp.StreamableHTTPOptions{
		// Stream SSE rather than buffer to JSON — claude-agent-acp's
		// MCP client expects the streaming variant and would otherwise
		// receive results out-of-order on long calls.
		JSONResponse: false,
		// Stateful: the streamable spec uses Mcp-Session-Id headers
		// to correlate POST/GET pairs and resume after disconnect.
		Stateless:      false,
		SessionTimeout: mcpSessionTimeout,
	})

	p.dialUpstream(ctx)
	return p
}

// dialUpstream opens the long-lived MCP client session to the upstream
// telemetry server and caches its tool list. Failures are logged and
// leave the proxy in a UI-tools-only state. Bounded by
// upstreamDialTimeout so a stuck upstream can't hang process startup.
//
// The parent context is the caller's startup context (threaded from
// RegisterRoutes ← initRouter ← createHTTPServer); this way the dial
// is cancelled if the OTel host tears the gateway down before the
// dial completes.
func (p *MCPProxy) dialUpstream(parent context.Context) {
	if p.upstreamURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, upstreamDialTimeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    mcpServerName + "-upstream-client",
		Version: mcpServerVersion,
	}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   p.upstreamURL,
		HTTPClient: http.DefaultClient,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		p.logger.Warn(
			"upstream MCP server unreachable; the AI gateway will serve UI tools only",
			zap.String("url", p.upstreamURL),
			zap.Error(err),
		)
		return
	}
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		p.logger.Warn(
			"upstream MCP server reachable but tools/list failed; the AI gateway will serve UI tools only",
			zap.String("url", p.upstreamURL),
			zap.Error(err),
		)
		return
	}
	p.upstream = session
	p.upstreamTools = listed.Tools
	p.logger.Info(
		"upstream MCP tools cached",
		zap.String("url", p.upstreamURL),
		zap.Int("tool_count", len(p.upstreamTools)),
	)
}

// Close releases the upstream MCP client session. Safe to call when no
// upstream connection was established (e.g. the initial dial failed) —
// idempotent, no-op in that case.
func (p *MCPProxy) Close() error {
	if p.upstream == nil {
		return nil
	}
	err := p.upstream.Close()
	p.upstream = nil
	return err
}

// sessionIDContextKey is the context key the path-extraction step uses
// to pass the AG-UI session id to serverForRequest. Using a context key
// (rather than re-parsing the URL inside serverForRequest) decouples the
// proxy from the URL shape — if the routing layer ever changes (e.g.
// query param, header), only ServeHTTP changes.
type sessionIDContextKey struct{}

// ServeHTTP is the entry point the route registration mounts. It peels
// the AG-UI session id off the path, rewrites the URL so the wrapped
// MCP handler sees its own clean root, and forwards.
//
// Path layout assumption:
//
//	/api/ai/mcp/<sessionId>/<mcp-subpath>
//
// where <mcp-subpath> may be empty (the streamable handler accepts both
// POST / and GET / as its protocol endpoints; trailing path is unused).
func (p *MCPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remainder, ok := strings.CutPrefix(r.URL.Path, routeMCPPrefix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(remainder, "/", 2)
	sessionID := parts[0]
	if sessionID == "" {
		http.Error(w, "session id is required in the URL", http.StatusBadRequest)
		return
	}

	// Rewrite r.URL.Path to "/" + remainder so the wrapped MCP handler
	// sees the root of its own mount. Clone the request so we don't
	// mutate the caller's URL, which other middleware further up the
	// stack might still be inspecting.
	rewritten := r.Clone(context.WithValue(r.Context(), sessionIDContextKey{}, sessionID))
	if len(parts) == 2 && parts[1] != "" {
		rewritten.URL.Path = "/" + parts[1]
	} else {
		rewritten.URL.Path = "/"
	}

	p.handler.ServeHTTP(w, rewritten)
}

// serverForRequest is the StreamableHTTPHandler's getServer callback.
// Returns a freshly-populated *mcp.Server scoped to the session id we
// stashed in the request context during ServeHTTP. An empty/unknown
// session id yields an empty server (no tools advertised); this is the
// graceful-degrade case for stray requests, not a normal flow.
func (p *MCPProxy) serverForRequest(r *http.Request) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    mcpServerName,
		Version: mcpServerVersion,
	}, nil)

	sessionID, _ := r.Context().Value(sessionIDContextKey{}).(string)
	if sessionID == "" {
		return srv
	}

	for _, raw := range p.ctxTools.GetContextualToolsForSession(sessionID) {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		if name == "" {
			continue
		}
		description, _ := tool["description"].(string)

		// MCP requires InputSchema to be a non-nil JSON object with
		// type:"object". Frontends usually provide a JSON Schema in the
		// `parameters` field; coerce missing/wrong shapes to a
		// permissive empty-object schema so a single malformed tool
		// can't take down session-scope registration.
		schema := normalizeUIToolSchema(tool["parameters"])

		if err := safeAddTool(srv, &mcp.Tool{
			Name:        name,
			Description: description,
			InputSchema: schema,
		}, p.uiToolHandler(sessionID, name)); err != nil {
			p.logger.Warn(
				"skipping malformed UI tool",
				zap.String("session_id", sessionID),
				zap.String("tool", name),
				zap.Error(err),
			)
		}
	}

	// Layer the cached upstream telemetry tools on top of the UI tools.
	// We register a fresh handler per session-scoped server because
	// each handler closes over the upstream client session; the closure
	// is the same shape but keeping registration per-server avoids
	// sharing handler instances across goroutines unnecessarily.
	for _, tool := range p.upstreamTools {
		if tool == nil || tool.Name == "" {
			continue
		}
		// On name collision UI tools win — the frontend explicitly
		// declared them for this turn, so the operator's intent is
		// stronger than the static upstream registration.
		if uiToolNameRegistered(p.ctxTools.GetContextualToolsForSession(sessionID), tool.Name) {
			p.logger.Debug(
				"upstream tool shadowed by a UI tool of the same name",
				zap.String("session_id", sessionID),
				zap.String("tool", tool.Name),
			)
			continue
		}
		if err := safeAddTool(srv, tool, p.upstreamToolHandler(tool.Name)); err != nil {
			p.logger.Warn(
				"skipping upstream tool with invalid schema",
				zap.String("session_id", sessionID),
				zap.String("tool", tool.Name),
				zap.Error(err),
			)
		}
	}

	return srv
}

// uiToolNameRegistered reports whether the per-session UI tool list
// already contains a tool with this name. Used by serverForRequest to
// skip upstream tools that would otherwise shadow a UI tool of the same
// name — see the collision rule in that function.
func uiToolNameRegistered(uiTools []any, name string) bool {
	for _, raw := range uiTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if n, _ := tool["name"].(string); n == name {
			return true
		}
	}
	return false
}

// upstreamToolHandler returns an MCP ToolHandler used by serverForRequest
// when registering upstream telemetry tools. Thin adapter — actual
// forwarding logic is in forwardToUpstream so the ACP transport can
// reuse it.
func (p *MCPProxy) upstreamToolHandler(toolName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.forwardToUpstream(ctx, toolName, req.Params.Arguments)
	}
}

// forwardToUpstream sends a tools/call to the cached upstream client
// session and returns the result verbatim. The agent sees the upstream
// tool as if it were registered on the gateway directly, but every
// invocation flows through this code path so the gateway can wrap it
// with tracing, auth, or rate limiting later.
func (p *MCPProxy) forwardToUpstream(ctx context.Context, toolName string, rawArgs json.RawMessage) (*mcp.CallToolResult, error) {
	if p.upstream == nil {
		return errorResult("upstream MCP server is not connected"), nil
	}
	// CallToolParams.Arguments is `any` (the SDK re-marshals on the
	// way out), so we have to convert from RawMessage here.
	var args any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return errorResult(fmt.Sprintf("invalid JSON arguments for upstream tool %q: %v", toolName, err)), nil
		}
	}
	return p.upstream.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

// uiToolHandler returns an MCP ToolHandler used by serverForRequest when
// registering UI tools on the per-session *mcp.Server (the HTTP
// transport). It's a thin adapter that unpacks the SDK's CallToolRequest
// and forwards to dispatchUITool, which contains the actual SSE-emit
// logic shared with the ACP transport.
func (p *MCPProxy) uiToolHandler(sessionID, toolName string) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.dispatchUITool(sessionID, toolName, req.Params.Arguments), nil
	}
}

// dispatchUITool fires the TOOL_CALL_* lifecycle for a UI tool on the
// per-session SSE stream and returns a synthetic ack. Used by both the
// HTTP path (via uiToolHandler) and the ACP path (via the mcp/message
// dispatcher in mcp_acp_dispatch.go). The dispatch is fire-and-forget
// at the LLM layer — the browser is the real executor and we don't
// wait for its result, just hand the agent a non-error CallToolResult
// so its tool-call loop can progress.
//
// rawArgs is the JSON bytes the agent sent (req.Params.Arguments for
// HTTP, params["arguments"] for ACP). It's re-unmarshalled here into
// `any` so EmitContextualToolCall can serialize it back as a delta
// string; bypassing the round-trip would be faster but would couple
// this layer to the streaming client's wire format.
func (p *MCPProxy) dispatchUITool(sessionID, toolName string, rawArgs json.RawMessage) *mcp.CallToolResult {
	sc := p.streams.Get(sessionID)
	if sc == nil {
		return errorResult(fmt.Sprintf("no active chat session for sessionId %q", sessionID))
	}
	var args any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return errorResult(fmt.Sprintf("invalid JSON arguments for tool %q: %v", toolName, err))
		}
	}
	sc.EmitContextualToolCall(newMCPToolCallID(toolName), toolName, args)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: fmt.Sprintf("ui tool %q dispatched to the browser", toolName),
		}},
	}
}

// errorResult is a small constructor for the IsError CallToolResult
// shape we return from internal dispatch failures. Used so we don't
// repeat the `{IsError: true, Content: [TextContent{}]}` boilerplate.
func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// normalizeUIToolSchema coerces whatever the frontend put in `parameters`
// into a JSON-object schema MCP will accept. Anything non-conforming
// degrades to `{"type":"object"}` so registration succeeds; the alternative
// would be to reject the tool entirely on a schema typo from the frontend,
// which is harsher than the gateway's responsibility for "advertise what
// you can, log what you can't."
func normalizeUIToolSchema(raw any) map[string]any {
	if m, ok := raw.(map[string]any); ok {
		if typ, _ := m["type"].(string); typ == "object" {
			return m
		}
	}
	return map[string]any{"type": "object"}
}

// safeAddTool wraps Server.AddTool's panic-on-invalid-schema behaviour
// in a recover() so one bad tool entry can't bring down the per-session
// server. The SDK chose panics for "the tool author made a mistake"
// errors; for us "the tool author" is the frontend, which we can't
// trust to always send valid schemas, so we have to translate panics
// into skip-this-tool decisions.
func safeAddTool(s *mcp.Server, t *mcp.Tool, h mcp.ToolHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	s.AddTool(t, h)
	return nil
}

// mcpToolCallIDSeq is a process-wide monotonic counter appended to
// generated tool call ids so two MCP tool dispatches within the same
// nanosecond don't collide.
var mcpToolCallIDSeq atomic.Uint64

// newMCPToolCallID produces a stable per-process unique identifier for
// a TOOL_CALL_* event group. Browser code only treats the value as
// opaque, so the format only matters for log correlation: name first
// makes call sites readable; nanos + counter guarantees uniqueness.
func newMCPToolCallID(name string) string {
	return fmt.Sprintf("%s-%d-%d", name, time.Now().UnixNano(), mcpToolCallIDSeq.Add(1))
}
