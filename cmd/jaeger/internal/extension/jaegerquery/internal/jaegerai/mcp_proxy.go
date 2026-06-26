// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// routeMCPPrefix is the URL prefix the MCP proxy mounts at. Everything
// under it is dispatched to the wrapped streamable HTTP handler — but
// only after we peel off the AG-UI session id from the next path
// segment, which is what scopes the per-session UI tool list.
const routeMCPPrefix = "/api/ai/mcp/"

// tracerName is the OTel tracer name the MCP proxy uses for its dispatch
// spans. The "jaeger.ai_gateway" namespace is parallel to "jaeger.mcp"
// (used by the jaegermcp extension) so operators can filter by emitting
// component in queries.
const tracerName = "jaeger.ai_gateway"

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
	tracer   trace.Tracer
	ctxTools *ContextualToolsStore
	streams  *SessionStreams
	handler  *mcp.StreamableHTTPHandler

	// basePath is the normalized jaeger-query base path (e.g. "/jaeger"
	// or ""); the gateway mounts its routes at <basePath>+routeMCPPrefix
	// and announceMCPServers must embed the same prefix in the URL it
	// hands to agents, or they 404 against /api/ai/mcp/... when the
	// operator runs behind a non-empty basePath.
	basePath string

	// upstreamURL is the HTTP MCP endpoint we dial for telemetry tools.
	// Set from upstreamMCPURL by NewMCPProxy; overridable via the
	// package-private newMCPProxyWithUpstream so tests can point at a
	// mock server.
	upstreamURL string

	// upstreamMu guards upstream + upstreamTools because the dial runs
	// in a background goroutine (see dialUpstreamWithRetry) — the
	// jaeger_mcp extension's Dependencies() forces it to start *after*
	// jaeger_query, so the first synchronous dial always fails. The
	// retry loop fills upstream + upstreamTools when jaeger_mcp comes
	// up; request goroutines read both fields under this lock.
	upstreamMu    sync.RWMutex
	upstream      *mcp.ClientSession
	upstreamTools []*mcp.Tool

	// acpConnections tracks live MCP-over-ACP callbacks the agent has
	// opened via mcp/connect. Keyed by connectionId; populated when
	// HandleConnect succeeds and drained by HandleDisconnect. See
	// mcp_acp_dispatch.go for the lifecycle.
	acpConnections *mcpACPConnections

	// uuidToSession maps the externally-visible per-turn UUID (the
	// last path segment in the announced MCP URL) to the internal
	// AG-UI session id assigned by the sidecar. The chat handler
	// allocates the UUID before session/new, registers the pair
	// once session/new returns, and unregisters on turn end. ServeHTTP
	// dials by UUID and looks up the real session id here; the agent
	// never sees the internal id directly.
	uuidMu        sync.RWMutex
	uuidToSession map[string]string
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
func NewMCPProxy(ctx context.Context, logger *zap.Logger, tracerProvider trace.TracerProvider, basePath string, ctxTools *ContextualToolsStore, streams *SessionStreams) *MCPProxy {
	return newMCPProxyWithUpstream(ctx, logger, tracerProvider, basePath, ctxTools, streams, upstreamMCPURL)
}

// newMCPProxyWithUpstream is the test-friendly constructor: it accepts
// an explicit upstream MCP URL instead of using the hardcoded default.
// Same behaviour otherwise — dials the upstream and primes the tool
// cache, degrading gracefully on dial failure.
//
// tracerProvider may be nil; the proxy falls back to the no-op provider
// in that case so tests don't need an OTel SDK in scope.
func newMCPProxyWithUpstream(ctx context.Context, logger *zap.Logger, tracerProvider trace.TracerProvider, basePath string, ctxTools *ContextualToolsStore, streams *SessionStreams, upstreamURL string) *MCPProxy {
	if logger == nil {
		logger = zap.NewNop()
	}
	if tracerProvider == nil {
		tracerProvider = nooptrace.NewTracerProvider()
	}
	p := &MCPProxy{
		logger:         logger,
		tracer:         tracerProvider.Tracer(tracerName),
		ctxTools:       ctxTools,
		streams:        streams,
		basePath:       basePath,
		upstreamURL:    upstreamURL,
		acpConnections: newMCPACPConnections(),
		uuidToSession:  make(map[string]string),
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

	// First attempt is synchronous so the happy path (and tests that
	// stand up an upstream before constructing the proxy) gets a fully
	// populated upstream cache by the time NewMCPProxy returns. If it
	// fails — the in-process case where jaeger_mcp's
	// Dependencies()-driven start follows jaeger_query — fall back to
	// a goroutine that retries on a fixed backoff until jaeger_mcp
	// comes up.
	if !p.tryDialUpstream(ctx) {
		go p.dialUpstreamWithRetry(ctx)
	}
	return p
}

// upstreamRetrySchedule is the backoff sequence dialUpstreamWithRetry
// walks through before giving up. Tuned for the in-process case where
// jaeger_mcp follows jaeger_query by ~10ms: the first wait is short so
// the upstream tools land before the first real chat request arrives,
// then the gaps widen for the "jaeger_mcp is genuinely misconfigured"
// failure mode. Total budget: ~7.7s. Anything longer would just delay
// the inevitable "give up and run UI-only" decision.
var upstreamRetrySchedule = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
}

// dialUpstreamWithRetry runs in a goroutine launched from
// newMCPProxyWithUpstream. It tries to open the MCP client session
// once immediately, and if that fails (jaeger_mcp not bound yet — the
// expected case during in-process startup because of the
// jaeger_mcp → jaeger_query Dependencies() edge), retries on a fixed
// backoff schedule. Each attempt is bounded by upstreamDialTimeout.
//
// The proxy starts in "UI tools only" mode and upgrades to "UI +
// upstream" as soon as a retry succeeds; in the common case the first
// chat request arrives long after the retry has filled the cache. If
// every retry fails, the proxy stays in UI-only mode and logs an
// explicit warning so operators can tell the difference between
// "jaeger_mcp is briefly slow" and "jaeger_mcp is genuinely down."
func (p *MCPProxy) dialUpstreamWithRetry(parent context.Context) {
	if p.upstreamURL == "" {
		return
	}
	for i, wait := range upstreamRetrySchedule {
		select {
		case <-parent.Done():
			return
		case <-time.After(wait):
		}
		p.logger.Debug(
			"retrying upstream MCP dial",
			zap.String("url", p.upstreamURL),
			zap.Int("attempt", i+2),
		)
		if p.tryDialUpstream(parent) {
			return
		}
	}
	p.logger.Warn(
		"upstream MCP server still unreachable after retries; the AI gateway will serve UI tools only",
		zap.String("url", p.upstreamURL),
		zap.Int("attempts", len(upstreamRetrySchedule)+1),
	)
}

// tryDialUpstream runs one dial + tools/list attempt and reports
// whether the proxy has a live upstream session afterwards. Returns
// true on success; on any failure the upstream state stays as it was
// (nil + empty) so callers can retry.
func (p *MCPProxy) tryDialUpstream(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, upstreamDialTimeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    mcpServerName + "-upstream-client",
		Version: mcpServerVersion,
	}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   p.upstreamURL,
		HTTPClient: http.DefaultClient,
		// The upstream MCP server (jaeger_mcp) never sends
		// server-initiated messages back to this client — every
		// dispatch is a request/response tools/call. Disabling the
		// standalone SSE stream avoids two related problems:
		//   1. jaeger_mcp's StreamableHTTPHandler.SessionTimeout
		//      (5min) evicts the session after that idle window;
		//      with a standalone SSE stream open, the eviction
		//      surfaces via a stale-SSE-reconnect path that wraps
		//      ErrSessionMissing with `%v` instead of `%w`
		//      (see go-sdk's streamable.go:2031), breaking
		//      errors.Is() detection on our side.
		//   2. The standalone SSE GET is an open file descriptor
		//      and a goroutine for the lifetime of the process —
		//      free of charge if we needed it, dead weight if not.
		// When the session evicts during long idle gaps, the next
		// POST tools/call still returns 404 — but the POST path
		// uses `%w` (streamable.go:2060), so the existing
		// errors.Is(err, mcp.ErrSessionMissing) check in
		// forwardToUpstream catches it and triggers reconnect.
		DisableStandaloneSSE: true,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		p.logger.Debug(
			"upstream MCP dial failed (will retry)",
			zap.String("url", p.upstreamURL),
			zap.Error(err),
		)
		return false
	}
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		p.logger.Debug(
			"upstream MCP reachable but tools/list failed (will retry)",
			zap.String("url", p.upstreamURL),
			zap.Error(err),
		)
		return false
	}
	p.upstreamMu.Lock()
	p.upstream = session
	p.upstreamTools = listed.Tools
	tools := make([]string, 0, len(listed.Tools))
	for _, t := range listed.Tools {
		if t != nil {
			tools = append(tools, t.Name)
		}
	}
	p.upstreamMu.Unlock()
	p.logger.Info(
		"upstream MCP tools cached",
		zap.String("url", p.upstreamURL),
		zap.Int("tool_count", len(listed.Tools)),
		zap.Strings("tools", tools),
	)
	return true
}

// isUpstreamSessionLost reports whether err indicates the upstream MCP
// session has been evicted and the cached client needs to be re-dialed.
//
// The straightforward case is errors.Is(err, mcp.ErrSessionMissing) —
// the Go SDK wraps the 404-from-POST path with %w so the chain is
// preserved. We also fall back to a substring match because the SDK's
// standalone-SSE-reconnect path (streamable.go:2031) wraps the inner
// ErrSessionMissing with `%v`, breaking errors.Is detection. The
// substring "session not found" is the leaf error text from
// mcp.ErrSessionMissing — cheap to match, and false positives would
// at worst trigger an unnecessary re-dial.
func isUpstreamSessionLost(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, mcp.ErrSessionMissing) {
		return true
	}
	return strings.Contains(err.Error(), "session not found")
}

// reconnectUpstream tears down the current upstream session and dials
// fresh. Used when forwardToUpstream sees ErrSessionMissing — the
// upstream evicted the session during a long idle gap, so the cached
// client now holds a stale session id and every call returns 404.
// One synchronous dial attempt is enough here; if it fails the caller
// surfaces the original error to the agent (no retry loop, because we
// don't want to wait seconds in the middle of a tool call).
func (p *MCPProxy) reconnectUpstream(ctx context.Context) bool {
	p.upstreamMu.Lock()
	if p.upstream != nil {
		_ = p.upstream.Close()
		p.upstream = nil
		p.upstreamTools = nil
	}
	p.upstreamMu.Unlock()
	return p.tryDialUpstream(ctx)
}

// upstreamSession returns the live upstream MCP client session under a
// read lock so request goroutines and the background dialer can't race
// on the field. nil means no upstream is connected (yet).
func (p *MCPProxy) upstreamSession() *mcp.ClientSession {
	p.upstreamMu.RLock()
	defer p.upstreamMu.RUnlock()
	return p.upstream
}

// upstreamToolList returns the cached upstream tools list under a read
// lock. The slice is the live cache — callers must not mutate.
func (p *MCPProxy) upstreamToolList() []*mcp.Tool {
	p.upstreamMu.RLock()
	defer p.upstreamMu.RUnlock()
	return p.upstreamTools
}

// Close releases the upstream MCP client session. Safe to call when no
// upstream connection was established (e.g. every retry failed) —
// idempotent, no-op in that case.
func (p *MCPProxy) Close() error {
	p.upstreamMu.Lock()
	session := p.upstream
	p.upstream = nil
	p.upstreamMu.Unlock()
	if session == nil {
		return nil
	}
	return session.Close()
}

// NewSessionUUID mints a random 128-bit token used as the external
// identifier in the announced MCP URL. The chat handler calls this
// before sending session/new so the URL can be embedded in mcpServers
// without knowing the sidecar-allocated session id yet — once the
// sidecar returns its id, RegisterUUIDForSession ties the two together.
// crypto/rand is overkill for an internal correlation token but trivial
// to use and removes any "is this guessable?" concern.
func NewSessionUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand never fails on Linux; fall back to a timestamp
		// so we never panic mid-turn on the off chance it does.
		ts := time.Now().UnixNano()
		return fmt.Sprintf("fallback-%016x", ts)
	}
	return hex.EncodeToString(b[:])
}

// RegisterUUIDForSession stores the uuid → sessionID mapping. Called by
// the chat handler exactly once per turn, right after session/new
// returns. Empty inputs are no-ops so callers can pass through pre-init
// state without guarding.
func (p *MCPProxy) RegisterUUIDForSession(uuid, sessionID string) {
	if uuid == "" || sessionID == "" {
		return
	}
	p.uuidMu.Lock()
	p.uuidToSession[uuid] = sessionID
	p.uuidMu.Unlock()
}

// UnregisterUUID drops the mapping for uuid. Called from the chat
// handler's defer block on turn end so the map doesn't accumulate
// entries across the gateway's lifetime. Idempotent.
func (p *MCPProxy) UnregisterUUID(uuid string) {
	if uuid == "" {
		return
	}
	p.uuidMu.Lock()
	delete(p.uuidToSession, uuid)
	p.uuidMu.Unlock()
}

// resolveSessionFromUUID looks up the internal AG-UI session id given
// the external UUID the agent dialed. Empty result means the UUID is
// unknown (already expired, never registered, or malicious).
func (p *MCPProxy) resolveSessionFromUUID(uuid string) string {
	p.uuidMu.RLock()
	defer p.uuidMu.RUnlock()
	return p.uuidToSession[uuid]
}

// sessionIDContextKey is the context key the path-extraction step uses
// to pass the AG-UI session id to serverForRequest. Using a context key
// (rather than re-parsing the URL inside serverForRequest) decouples the
// proxy from the URL shape — if the routing layer ever changes (e.g.
// query param, header), only ServeHTTP changes.
type sessionIDContextKey struct{}

// ServeHTTP is the entry point the route registration mounts. It peels
// the per-turn UUID off the path, resolves it to the internal AG-UI
// session id via the uuid→session map, rewrites the URL so the wrapped
// MCP handler sees its own clean root, and forwards.
//
// Path layout assumption:
//
//	/api/ai/mcp/<uuid>/<mcp-subpath>
//
// where <mcp-subpath> may be empty (the streamable handler accepts both
// POST / and GET / as its protocol endpoints; trailing path is unused).
// An unknown UUID yields 404 — the agent must have a valid in-flight
// session to reach the per-session tool catalogue.
func (p *MCPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remainder, ok := strings.CutPrefix(r.URL.Path, routeMCPPrefix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(remainder, "/", 2)
	uuid := parts[0]
	if uuid == "" {
		http.Error(w, "session uuid is required in the URL", http.StatusBadRequest)
		return
	}
	sessionID := p.resolveSessionFromUUID(uuid)
	if sessionID == "" {
		http.Error(w, "unknown session uuid", http.StatusNotFound)
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

		// The wire-level name is UIToolPrefix-namespaced to avoid
		// colliding with built-in telemetry tool names; the handler is
		// still keyed by the raw frontend name so ctxTools lookups
		// (which store raw names) succeed without an extra strip step.
		if err := safeAddTool(srv, &mcp.Tool{
			Name:        applyUIToolPrefix(name),
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
	// Each handler closes over (sessionID, toolName) and re-resolves
	// the live upstream client through p.upstreamSession() on every
	// call, so reconnects (see reconnectUpstream) take effect mid-turn
	// without re-registering the per-session server.
	for _, tool := range p.upstreamToolList() {
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
		if err := safeAddTool(srv, tool, p.upstreamToolHandler(sessionID, tool.Name)); err != nil {
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
func (p *MCPProxy) upstreamToolHandler(sessionID, toolName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.forwardToUpstream(ctx, sessionID, toolName, req.Params.Arguments)
	}
}

// forwardToUpstream sends a tools/call to the cached upstream client
// session and returns the result verbatim. Every invocation also emits
// the full TOOL_CALL_START / ARGS / RESULT / END lifecycle on the
// session's SSE stream so the browser observes upstream tool activity
// the same way it observes UI-tool dispatch — the gateway is the
// single source of TOOL_CALL_* events regardless of which transport
// the agent used to reach it.
//
// sessionID is used to look up the SSE stream; if empty (no session
// known) or the streams registry isn't wired, the forward still works
// but no SSE is emitted — this matches the test-time degraded mode.
func (p *MCPProxy) forwardToUpstream(ctx context.Context, sessionID, toolName string, rawArgs json.RawMessage) (*mcp.CallToolResult, error) {
	ctx, span := p.tracer.Start(
		ctx,
		"mcp_proxy.forward_upstream",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			otelsemconv.GenAIOperationNameExecuteTool,
			otelsemconv.GenAIToolName(toolName),
			otelsemconv.McpSessionID(sessionID),
		),
	)
	defer span.End()

	session := p.upstreamSession()
	if session == nil {
		const msg = "upstream MCP server is not connected"
		span.SetStatus(codes.Error, msg)
		span.SetAttributes(otelsemconv.ErrorType("upstream_unavailable"))
		return errorResult(msg), nil
	}
	// CallToolParams.Arguments is `any` (the SDK re-marshals on the
	// way out), so we have to convert from RawMessage here.
	var args any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			msg := fmt.Sprintf("invalid JSON arguments for upstream tool %q: %v", toolName, err)
			span.RecordError(err)
			span.SetStatus(codes.Error, msg)
			span.SetAttributes(otelsemconv.ErrorType("invalid_arguments"))
			return errorResult(msg), nil
		}
	}
	params := &mcp.CallToolParams{Name: toolName, Arguments: args}
	result, err := session.CallTool(ctx, params)
	// jaeger_mcp's StreamableHTTPHandler evicts idle sessions on its
	// own SessionTimeout, after which our cached upstream client holds
	// a stale session id and the next call gets back ErrSessionMissing
	// (HTTP 404 from the upstream). Detect that case and transparently
	// re-dial + retry once so a quiet gap between chat turns doesn't
	// poison the next request.
	if isUpstreamSessionLost(err) {
		p.logger.Info(
			"upstream MCP session was evicted; re-dialing and retrying",
			zap.String("tool", toolName),
		)
		span.AddEvent("upstream_session_evicted_redialing")
		_ = p.reconnectUpstream(ctx)
		if newSession := p.upstreamSession(); newSession != nil {
			result, err = newSession.CallTool(ctx, params)
		}
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else if result != nil && result.IsError {
		// Distinguish transport failures (err != nil) from tool-level
		// errors (result.IsError) so dashboards can split them.
		span.SetAttributes(otelsemconv.ErrorType("tool_error"))
	}

	// Emit SSE even when the upstream call errored, so the browser
	// receives a complete START/END pair instead of a half-open call
	// card stuck "in progress". A transport-level error becomes a
	// RESULT with the error string and IsError flagged downstream.
	if sc := p.streamsForSession(sessionID); sc != nil {
		toolCallID := newMCPToolCallID(toolName)
		resultText := upstreamResultText(result, err)
		sc.EmitUpstreamToolCall(toolCallID, toolName, args, resultText)
	}

	return result, err
}

// streamsForSession returns the streamingClient for sessionID, or nil
// if the streams registry isn't wired (tests) or the session is
// unknown. Wrapping the nil check in one place keeps the SSE-emission
// call sites readable.
func (p *MCPProxy) streamsForSession(sessionID string) *streamingClient {
	if p.streams == nil || sessionID == "" {
		return nil
	}
	return p.streams.Get(sessionID)
}

// upstreamResultText flattens an MCP CallToolResult's Content blocks
// into a single string, joining text parts with "\n". On a transport
// error (result == nil, err != nil) the error message is used so the
// browser still sees a meaningful RESULT body.
func upstreamResultText(result *mcp.CallToolResult, err error) string {
	if result == nil {
		if err != nil {
			return err.Error()
		}
		return ""
	}
	parts := make([]string, 0, len(result.Content))
	for _, c := range result.Content {
		if t, ok := c.(*mcp.TextContent); ok && t.Text != "" {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// uiToolHandler returns an MCP ToolHandler used by serverForRequest when
// registering UI tools on the per-session *mcp.Server (the HTTP
// transport). It's a thin adapter that unpacks the SDK's CallToolRequest
// and forwards to dispatchUITool, which contains the actual SSE-emit
// logic shared with the ACP transport.
func (p *MCPProxy) uiToolHandler(sessionID, toolName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.dispatchUITool(ctx, sessionID, toolName, req.Params.Arguments), nil
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
//
// A span is started under tracerName so operators can see the
// gateway-side fan-out next to the sidecar's gateway_mcp.call_tool
// span. Attributes use the OTel GenAI semconv (matching jaeger_mcp's
// middleware) so cross-component queries work without per-component
// rename tables.
func (p *MCPProxy) dispatchUITool(ctx context.Context, sessionID, toolName string, rawArgs json.RawMessage) *mcp.CallToolResult {
	_, span := p.tracer.Start(
		ctx,
		"mcp_proxy.dispatch_ui_tool",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			otelsemconv.GenAIOperationNameExecuteTool,
			otelsemconv.GenAIToolName(toolName),
			otelsemconv.McpSessionID(sessionID),
		),
	)
	defer span.End()

	sc := p.streams.Get(sessionID)
	if sc == nil {
		msg := fmt.Sprintf("no active chat session for sessionId %q", sessionID)
		span.SetStatus(codes.Error, msg)
		span.SetAttributes(otelsemconv.ErrorType("session_not_active"))
		return errorResult(msg)
	}
	var args any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			msg := fmt.Sprintf("invalid JSON arguments for tool %q: %v", toolName, err)
			span.RecordError(err)
			span.SetStatus(codes.Error, msg)
			span.SetAttributes(otelsemconv.ErrorType("invalid_arguments"))
			return errorResult(msg)
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
