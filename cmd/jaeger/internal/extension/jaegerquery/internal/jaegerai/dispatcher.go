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
// gateway logs the request and returns a placeholder response; PR2 will
// replace the placeholder with a real round-trip to the AG-UI client.
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

// newDispatcher returns an acp.MethodHandler that routes inbound
// JSON-RPC from the sidecar:
//   - session/update → streamingClient.SessionUpdate (writes agent text
//     and tool-progress markers into the chat HTTP response).
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
func newDispatcher(client *streamingClient, store *ContextualToolsStore, logger *zap.Logger) acp.MethodHandler {
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
// fire-and-forget side effects. The sidecar emits start_tool_call /
// update_tool_call session_update notifications around this call, which
// the streaming client renders as AG-UI TOOL_CALL_* SSE events for the
// browser to react to (navigate, render, etc.). The browser is not
// expected to return a tool result — UI tools are commands, not queries
// — so the gateway acknowledges immediately and lets Gemini's agentic
// loop continue with a "tool dispatched" function response.
//
// The tool name arrives “UIToolPrefix“-namespaced (the gateway adds
// the prefix in handler.go before populating the contextual tools meta
// payload) and is stripped back here so downstream consumers and logs
// see the original frontend-supplied name. Callers that pre-date the
// prefix pass the name through unchanged and only log a warning, so
// the strip is safe to deploy ahead of the meta-side prefix-add work.
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
		logger.Warn("contextual tool name missing UI prefix; passing through unchanged",
			zap.String("tool", req.Name),
			zap.String("expected_prefix", UIToolPrefix),
		)
	}
	if !sessionHasTool(store, req.SessionID, req.Name) {
		return extToolCallResponse{}, acp.NewInvalidParams(map[string]any{
			"error": fmt.Sprintf("contextual tool %q not registered for session %q", req.Name, req.SessionID),
		})
	}
	logger.Info("contextual tool call dispatched (fire-and-forget)",
		zap.String("session_id", req.SessionID),
		zap.String("tool", req.Name),
		zap.String("prefixed_tool", originalName),
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
