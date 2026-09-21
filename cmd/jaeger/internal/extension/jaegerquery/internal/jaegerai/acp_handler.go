// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/version"
)

// ExtMethodJaegerToolCall is the ACP extension method the sidecar invokes
// when Gemini requests a contextual (frontend-supplied) tool call.
//
// Deprecated: use standard MCP tools/call over HTTP or MCP-over-ACP instead.
const ExtMethodJaegerToolCall = "_meta/jaegertracing.io/tools/call"

// UIToolPrefix is the namespace the gateway prepends to every contextual
// tool name before advertising it to the sidecar (and through it to
// Gemini). The prefix prevents a frontend-supplied tool from shadowing a
// built-in Jaeger MCP tool with the same name (e.g. "search_traces") and
// is stripped here when the sidecar relays the call back, so the
// downstream AG-UI client receives the original frontend name.
const UIToolPrefix = "ui_"

// ContextualToolsMetaKey is the namespaced key the gateway uses under
// NewSessionRequest._meta to ship the frontend-provided AG-UI tools
// snapshot to the sidecar. The value at this key has shape
// {"tools": [<aguitypes.Tool>, ...]}. The sidecar (sidecar_helpers.py
// reads this same key) registers those tools with the LLM so the model
// can decide when to invoke them; the gateway holds the same snapshot
// in ContextualToolsStore for any callbacks that come back.
//
// Deprecated: use standard MCP tools/list instead.
const ContextualToolsMetaKey = "jaegertracing.io/contextual-tools"

// extToolCallRequest is the payload the sidecar sends with
// ExtMethodJaegerToolCall. The gateway treats it as opaque except for
// logging — Args is left as RawMessage to avoid an extra round-trip
// through map[string]any.
type extToolCallRequest struct {
	SessionID string          `json:"sessionId"`
	Name      string          `json:"name"`
	Args      json.RawMessage `json:"args,omitempty"`
}

// extToolCallResponse is what the gateway returns to the sidecar after a
// contextual tool dispatch. Contextual tools are fire-and-forget side
// effects (the browser executes them, no result is round-tripped back),
// so the gateway always returns an acknowledgement. The Result/IsError
// shape mirrors MCP's CallToolResult so the sidecar can feed it to the
// LLM unchanged as the function response for the dispatched call.
type extToolCallResponse struct {
	Result  any  `json:"result"`
	IsError bool `json:"isError,omitempty"`
}

type mcpConnection struct {
	connID     acp.UnstableMcpConnectionId
	mcpRouteID string
}

type mcpConnRegistry struct {
	mu    sync.RWMutex
	conns map[acp.UnstableMcpConnectionId]*mcpConnection
}

func newMcpConnRegistry() *mcpConnRegistry {
	return &mcpConnRegistry{
		conns: make(map[acp.UnstableMcpConnectionId]*mcpConnection),
	}
}

func (r *mcpConnRegistry) add(connID acp.UnstableMcpConnectionId, mcpRouteID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[connID] = &mcpConnection{connID: connID, mcpRouteID: mcpRouteID}
}

func (r *mcpConnRegistry) get(connID acp.UnstableMcpConnectionId) *mcpConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[connID]
}

func (r *mcpConnRegistry) remove(connID acp.UnstableMcpConnectionId) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, connID)
}

type acpHandler struct {
	client   *streamingClient
	store    *ContextualToolsStore
	turns    *turnRegistry
	upstream UpstreamCaller
	conns    *mcpConnRegistry
	tracer   oteltrace.Tracer
	logger   *zap.Logger
}

// ACPHandlerOption configures optional parameters for newACPHandler.
type ACPHandlerOption func(*acpHandler)

// WithACPHandlerTurns injects the turnRegistry for MCP session resolution.
func WithACPHandlerTurns(turns *turnRegistry) ACPHandlerOption {
	return func(h *acpHandler) {
		h.turns = turns
	}
}

// WithACPHandlerUpstream injects the upstream caller for telemetry tool routing.
func WithACPHandlerUpstream(upstream UpstreamCaller) ACPHandlerOption {
	return func(h *acpHandler) {
		h.upstream = upstream
	}
}

// WithACPHandlerTracer injects an OpenTelemetry tracer.
func WithACPHandlerTracer(tracer oteltrace.Tracer) ACPHandlerOption {
	return func(h *acpHandler) {
		h.tracer = tracer
	}
}

// newACPHandler returns an acp.MethodHandler that routes inbound JSON-RPC from the sidecar:
//   - session/update → streamingClient.SessionUpdate
//   - session/request_permission → streamingClient.RequestPermission
//   - mcp/connect → establishes an MCP-over-ACP connection for an active turn
//   - mcp/message → dispatches inner MCP requests (initialize, tools/list, tools/call, ping)
//   - mcp/disconnect → terminates an MCP-over-ACP connection
//   - ExtMethodJaegerToolCall → (deprecated) legacy contextual tool call
//   - anything else → MethodNotFound.
func newACPHandler(client *streamingClient, store *ContextualToolsStore, logger *zap.Logger, opts ...ACPHandlerOption) acp.MethodHandler {
	h := &acpHandler{
		client: client,
		store:  store,
		conns:  newMcpConnRegistry(),
		tracer: otel.GetTracerProvider().Tracer("jaeger.ai.mcp"),
		logger: logger,
	}
	for _, opt := range opts {
		opt(h)
	}

	return func(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
		switch method {
		case acp.ClientMethodSessionUpdate:
			var p acp.SessionNotification
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			return nil, toRequestError(h.client.SessionUpdate(ctx, p))

		case acp.ClientMethodSessionRequestPermission:
			var p acp.RequestPermissionRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			resp, err := h.client.RequestPermission(ctx, p)
			return resp, toRequestError(err)

		case acp.ClientMethodMcpConnect:
			return h.handleMcpConnect(ctx, params)

		case acp.ClientMethodMcpMessage:
			return h.handleMcpMessage(ctx, params)

		case acp.ClientMethodMcpDisconnect:
			return h.handleMcpDisconnect(ctx, params)

		case ExtMethodJaegerToolCall:
			return handleJaegerToolCall(params, h.store, h.logger)

		default:
			return nil, acp.NewMethodNotFound(method)
		}
	}
}

func (h *acpHandler) handleMcpConnect(_ context.Context, params json.RawMessage) (any, *acp.RequestError) {
	var req acp.UnstableConnectMcpRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
	}
	if req.AcpId == "" {
		return nil, acp.NewInvalidParams(map[string]any{"error": "acpId is required"})
	}
	mcpRouteID := string(req.AcpId)
	if h.turns == nil || h.turns.get(mcpRouteID) == nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("turn %q not found or inactive", mcpRouteID)})
	}
	connID := acp.UnstableMcpConnectionId(uuid.NewString())
	h.conns.add(connID, mcpRouteID)
	return acp.UnstableConnectMcpResponse{
		ConnectionId: connID,
	}, nil
}

func (h *acpHandler) handleMcpDisconnect(_ context.Context, params json.RawMessage) (any, *acp.RequestError) {
	var req acp.UnstableDisconnectMcpRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
	}
	if req.ConnectionId == "" {
		return nil, acp.NewInvalidParams(map[string]any{"error": "connectionId is required"})
	}
	h.conns.remove(req.ConnectionId)
	return acp.UnstableDisconnectMcpResponse{}, nil
}

func (h *acpHandler) handleMcpMessage(ctx context.Context, params json.RawMessage) (any, *acp.RequestError) {
	var req acp.UnstableMessageMcpRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
	}
	if req.ConnectionId == "" {
		return nil, acp.NewInvalidParams(map[string]any{"error": "connectionId is required"})
	}
	conn := h.conns.get(req.ConnectionId)
	if conn == nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("connectionId %q not found", req.ConnectionId)})
	}
	var turn *turnState
	if h.turns != nil {
		turn = h.turns.get(conn.mcpRouteID)
	}
	if turn == nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("turn %q closed or expired", conn.mcpRouteID)})
	}

	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    mcpServerName,
				"version": version.Get().GitVersion,
			},
		}, nil

	case methodListTools:
		var telemetryTools []*mcp.Tool
		if h.upstream != nil {
			listRes, err := h.upstream.ListTools(ctx)
			if err != nil {
				h.logger.Warn("failed to fetch upstream tools", zap.Error(err))
			} else if listRes != nil {
				telemetryTools = listRes.Tools
			}
		}
		merged := appendUITools(telemetryTools, turn, h.logger)
		return &mcp.ListToolsResult{Tools: merged}, nil

	case methodCallTool:
		toolName, _ := req.Params["name"].(string)
		if toolName == "" {
			return nil, acp.NewInvalidParams(map[string]any{"error": "tool name is required"})
		}
		var rawArgs json.RawMessage
		if argsVal, ok := req.Params["arguments"]; ok && argsVal != nil {
			rawArgs, _ = json.Marshal(argsVal)
		}

		if turnDeclaredUITool(turn, toolName) {
			res := dispatchUITool(ctx, turn, toolName, rawArgs, h.tracer, h.logger)
			return res, nil
		}

		res, err := forwardToUpstream(ctx, turn, toolName, rawArgs, func(callCtx context.Context) (*mcp.CallToolResult, error) {
			if h.upstream != nil {
				var unmarshaledArgs any
				if len(rawArgs) > 0 {
					_ = json.Unmarshal(rawArgs, &unmarshaledArgs)
				}
				return h.upstream.CallTool(callCtx, toolName, unmarshaledArgs)
			}
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("tool %q not found or upstream unavailable", toolName)}},
			}, nil
		}, h.tracer, h.logger)
		if err != nil {
			return nil, acp.NewInternalError(map[string]any{"error": err.Error()})
		}
		return res, nil

	case "ping":
		return map[string]any{}, nil

	default:
		return nil, acp.NewMethodNotFound(req.Method)
	}
}

// handleJaegerToolCall handles contextual tool dispatches as
// fire-and-forget today: the sidecar emits start_tool_call /
// update_tool_call session_update notifications around this call, which
// the streaming client renders as AG-UI TOOL_CALL_* SSE events for the
// browser, and the gateway acknowledges the ext_method immediately so
// Gemini's agentic loop continues with a "tool dispatched" function
// response.
//
// This is a transport-driven simplification, NOT a principled "results
// aren't useful" stance. Real tool results would be valuable — the LLM
// could use them in subsequent reasoning, and permission-style tools
// genuinely need a user answer fed back. The reason we don't carry them
// today is that the chat endpoint is HTTP+SSE, and SSE is
// unidirectional (server→client only): once the response stream is
// open, the browser has no in-band channel to push a tool result back
// while the ext_method waits.
//
// The full-fidelity fix is to switch the browser ↔ gateway transport
// to WebSocket, which gives bidirectional framing without inventing a
// non-AG-UI side endpoint. That's a larger change because WS adds
// infrastructure cost everywhere it lands — reverse proxies, load
// balancers, sticky-session routing across multiple Jaeger instances,
// idle timeouts, keepalive — none of which HTTP+SSE forces on the
// deployment. Switching is a follow-up tracked in the AI-gateway RFC;
// until then the fire-and-forget ack keeps the single-turn UX intact
// for command-shaped UI tools, which is the only category the current
// frontend exposes.
//
// The tool name arrives “UIToolPrefix“-namespaced — the gateway adds
// the prefix in handler.go before populating the contextual tools meta
// payload — and is stripped back here so downstream consumers and logs
// see the original frontend-supplied name. As a defensive compatibility
// shim, a sidecar that omits the prefix is not rejected: the unprefixed
// name is passed through unchanged and a warning is logged so any
// regressions are visible without breaking dispatches.
//
// After stripping, the dispatcher confirms the (unprefixed) name is
// present in the per-session contextual tools snapshot. A miss yields
// InvalidParams so a misbehaving sidecar or LLM cannot dispatch a tool
// the frontend never declared. nil store rejects every call as
// "no contextual tools registered" — useful for tests but also a safe
// default if Set/Delete were skipped for some reason.
func handleJaegerToolCall(params json.RawMessage, store *ContextualToolsStore, logger *zap.Logger) (extToolCallResponse, *acp.RequestError) {
	var req extToolCallRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return extToolCallResponse{}, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
	}
	if req.SessionID == "" {
		return extToolCallResponse{}, acp.NewInvalidParams(map[string]any{"error": "sessionId is required"})
	}
	if req.Name == "" {
		return extToolCallResponse{}, acp.NewInvalidParams(map[string]any{"error": "tool name is required"})
	}
	logger.Warn(
		"invoked deprecated ACP extension method _meta/jaegertracing.io/tools/call; migrate to MCP tools/call",
		zap.String("session_id", req.SessionID),
		zap.String("tool", req.Name),
	)
	originalName := req.Name
	if stripped, ok := strings.CutPrefix(req.Name, UIToolPrefix); ok {
		if stripped == "" {
			return extToolCallResponse{}, acp.NewInvalidParams(map[string]any{
				"error": fmt.Sprintf("tool name was only the UI prefix %q; expected %s<name>", UIToolPrefix, UIToolPrefix),
			})
		}
		req.Name = stripped
	} else {
		logger.Warn(
			"contextual tool name missing UI prefix; passing through unchanged",
			zap.String("tool", req.Name),
			zap.String("expected_prefix", UIToolPrefix),
		)
	}
	if !sessionHasTool(store, req.SessionID, req.Name) {
		return extToolCallResponse{}, acp.NewInvalidParams(map[string]any{
			"error": fmt.Sprintf("contextual tool %q not registered for session %q", req.Name, req.SessionID),
		})
	}
	// args is dropped from the Info-level record so logs don't carry
	// arbitrary user-provided payloads (potential PII, oversize entries,
	// noisy operator logs). The size is kept at Info for observability
	// — a "this tool was dispatched with N bytes of arguments" record is
	// useful and non-leaky. Full args are emitted only at Debug, where
	// operators must explicitly opt in.
	logger.Info(
		"contextual tool call dispatched (fire-and-forget)",
		zap.String("session_id", req.SessionID),
		zap.String("tool", req.Name),
		zap.String("prefixed_tool", originalName),
		zap.Int("args_size_bytes", len(req.Args)),
	)
	logger.Debug(
		"contextual tool call args",
		zap.String("session_id", req.SessionID),
		zap.String("tool", req.Name),
		zap.ByteString("args", req.Args),
	)
	return extToolCallResponse{
		Result:  map[string]any{"acknowledged": true},
		IsError: false,
	}, nil
}

// sessionHasTool reports whether the contextual tools snapshot for
// sessionID contains an entry whose `name` field equals toolName. The
// store stores the tools un-prefixed (matching what the frontend sent),
// so the caller passes the post-strip name. A nil store, a missing
// session, or a snapshot with no matching entry all return false.
func sessionHasTool(store *ContextualToolsStore, sessionID, toolName string) bool {
	if store == nil {
		return false
	}
	for _, tool := range store.GetContextualToolsForSession(sessionID) {
		entry, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := entry["name"].(string); ok && name == toolName {
			return true
		}
	}
	return false
}

// toRequestError converts an arbitrary client error into a
// *acp.RequestError. Returns nil when the input is nil so call sites
// can pass through the result of a fallible client call without an
// explicit “if err != nil“ guard. Existing *acp.RequestError values
// (so handlers can return precise error codes) are preserved; anything
// else is wrapped as InternalError.
func toRequestError(err error) *acp.RequestError {
	if err == nil {
		return nil
	}
	var re *acp.RequestError
	if errors.As(err, &re) {
		return re
	}
	return acp.NewInternalError(map[string]any{"error": fmt.Sprintf("client handler error: %v", err)})
}
