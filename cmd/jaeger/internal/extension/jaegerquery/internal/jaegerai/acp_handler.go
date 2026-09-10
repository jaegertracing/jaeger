// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

// ExtMethodJaegerToolCall is the ACP extension method the sidecar invokes
// when Gemini requests a contextual (frontend-supplied) tool call. The
// gateway strips the UIToolPrefix, validates the (post-strip) name against
// the per-session contextual tools snapshot, logs the dispatch, and
// returns an immediate `{acknowledged: true}` ack. The browser observes
// the call via the parallel TOOL_CALL_* SSE stream and performs the side
// effect locally — see handleJaegerToolCall for the fire-and-forget
// rationale.
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

// newACPHandler returns an acp.MethodHandler that routes inbound
// JSON-RPC from the sidecar:
//   - session/update → streamingClient.SessionUpdate (translates ACP
//     updates into typed AG-UI events — TEXT_MESSAGE_* for assistant
//     text and TOOL_CALL_* for tool-call lifecycle — and writes them as
//     SSE frames on the chat HTTP response).
//   - session/request_permission → streamingClient.RequestPermission
//     (always denies; we advertise no fs/terminal capability).
//   - ExtMethodJaegerToolCall → validate the contextual tool dispatch
//     against the per-session snapshot and acknowledge with a fire-and-
//     forget result; the browser executes the side effect on its own.
//   - anything else → MethodNotFound.
//
// store is consulted by handleJaegerToolCall to confirm the dispatched
// tool was registered by the frontend for this session; nil store is
// allowed for tests but rejects every contextual call as "not registered".
//
// The standard-method paths replicate the subset of acp_client_gen.go
// dispatch the gateway actually needs; we cannot reuse the SDK's
// hardcoded ClientSideConnection.handle because it returns MethodNotFound
// for our extension method. Client errors flow back through the nil-safe
// toRequestError so the dispatcher itself stays branchless on the
// non-malformed-params path.
func newACPHandler(client *streamingClient, store *ContextualToolsStore, logger *zap.Logger) acp.MethodHandler {
	return func(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
		switch method {
		case acp.ClientMethodSessionUpdate:
			var p acp.SessionNotification
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			return nil, toRequestError(client.SessionUpdate(ctx, p))

		case acp.ClientMethodSessionRequestPermission:
			var p acp.RequestPermissionRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			resp, err := client.RequestPermission(ctx, p)
			return resp, toRequestError(err)

		case ExtMethodJaegerToolCall:
			return handleJaegerToolCall(params, store, logger)

		default:
			return nil, acp.NewMethodNotFound(method)
		}
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
