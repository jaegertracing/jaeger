// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

// ExtMethodJaegerToolCall is the ACP extension method the sidecar invokes
// when Gemini requests a contextual (frontend-supplied) tool call. The
// gateway logs the request and returns a placeholder response; PR2 will
// replace the placeholder with a real round-trip to the AG-UI client.
const ExtMethodJaegerToolCall = "_meta/jaegertracing.io/tools/call"

// UITooLPrefix is the namespace the gateway prepends to every contextual
// tool name before advertising it to the sidecar (and through it to
// Gemini). The prefix prevents a frontend-supplied tool from shadowing a
// built-in Jaeger MCP tool with the same name (e.g. "search_traces") and
// is stripped here when the sidecar relays the call back, so the
// downstream AG-UI client receives the original frontend name.
const UIToolPrefix = "ui_"

// extToolCallRequest is the payload the sidecar sends with
// ExtMethodJaegerToolCall. The gateway treats it as opaque except for
// logging — Args is left as RawMessage to avoid an extra round-trip
// through map[string]any.
type extToolCallRequest struct {
	SessionID string          `json:"sessionId"`
	Name      string          `json:"name"`
	Args      json.RawMessage `json:"args,omitempty"`
}

// extToolCallResponse is what the gateway returns to the sidecar. The
// shape mirrors MCP's CallToolResult so swapping the placeholder for a
// real browser-relayed result in PR2 is a straight substitution.
type extToolCallResponse struct {
	Result  any    `json:"result"`
	IsError bool   `json:"isError,omitempty"`
	Note    string `json:"note,omitempty"`
}

// newDispatcher returns an acp.MethodHandler that routes inbound
// JSON-RPC from the sidecar:
//   - session/update → streamingClient.SessionUpdate (writes agent text
//     and tool-progress markers into the chat HTTP response).
//   - session/request_permission → streamingClient.RequestPermission
//     (always denies; we advertise no fs/terminal capability).
//   - ExtMethodJaegerToolCall → log and return placeholder; PR2 will
//     forward to the AG-UI client and return its result.
//   - anything else → MethodNotFound.
//
// The standard-method paths replicate the subset of acp_client_gen.go
// dispatch the gateway actually needs; we cannot reuse the SDK's
// hardcoded ClientSideConnection.handle because it returns MethodNotFound
// for our extension method. Client errors flow back through the nil-safe
// toRequestError so the dispatcher itself stays branchless on the
// non-malformed-params path.
func newDispatcher(client *streamingClient, logger *zap.Logger) acp.MethodHandler {
	return func(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
		switch method {
		case acp.ClientMethodSessionUpdate:
			var p acp.SessionNotification
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
			}
			return nil, toRequestError(client.SessionUpdate(ctx, p))

		case acp.ClientMethodSessionRequestPermission:
			var p acp.RequestPermissionRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
			}
			resp, err := client.RequestPermission(ctx, p)
			return resp, toRequestError(err)

		case ExtMethodJaegerToolCall:
			return handleJaegerToolCall(params, logger)

		default:
			return nil, acp.NewMethodNotFound(method)
		}
	}
}

// handleJaegerToolCall logs the contextual tool call the sidecar
// dispatched and returns a placeholder result so Gemini's agentic loop
// can continue without treating the call as a hard failure. PR2 will
// replace this with a real round-trip to the AG-UI-connected client.
//
// The tool name arrives “UIToolPrefix“-namespaced (the gateway adds
// the prefix when populating the contextual tools meta payload — see
// the TODO in handler.go) and is stripped back here so downstream
// consumers see the original frontend-supplied name. Callers that
// pre-date the prefix work pass the name through unchanged and only
// log a warning, so the strip is safe to deploy ahead of the meta-side
// prefix-add work.
func handleJaegerToolCall(params json.RawMessage, logger *zap.Logger) (any, *acp.RequestError) {
	var req extToolCallRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
	}
	originalName := req.Name
	if stripped, ok := strings.CutPrefix(req.Name, UIToolPrefix); ok {
		req.Name = stripped
	} else {
		logger.Warn("contextual tool name missing UI prefix; passing through unchanged",
			zap.String("tool", req.Name),
			zap.String("expected_prefix", UIToolPrefix),
		)
	}
	logger.Info("contextual tool call received from sidecar (AG-UI relay pending)",
		zap.String("session_id", req.SessionID),
		zap.String("tool", req.Name),
		zap.String("prefixed_tool", originalName),
		zap.ByteString("args", req.Args),
	)
	return extToolCallResponse{
		Result: nil,
		Note:   "tool logged, AG-UI relay not yet wired",
	}, nil
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
	return acp.NewInternalError(map[string]any{"error": err.Error()})
}
