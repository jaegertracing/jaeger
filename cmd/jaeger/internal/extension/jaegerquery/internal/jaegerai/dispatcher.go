// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

// ExtMethodJaegerToolCall is the ACP extension method the sidecar invokes
// when Gemini requests a contextual (frontend-supplied) tool call. The
// gateway logs the request and returns a placeholder response; PR2 will
// replace the placeholder with a real round-trip to the AG-UI client.
const ExtMethodJaegerToolCall = "_meta/jaegertracing.io/tools/call"

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
//   - session/update → streamingClient.SessionUpdate (so agent text and
//     tool-progress markers stream into the chat HTTP response).
//   - session/request_permission → streamingClient.RequestPermission
//     (always denies; we advertise no fs/terminal capability).
//   - ExtMethodJaegerToolCall → log and return placeholder; PR2 will
//     forward to the AG-UI client and return its result.
//   - anything else → MethodNotFound.
//
// The standard-method paths replicate the subset of acp_client_gen.go
// dispatch the gateway actually needs; we cannot reuse the SDK's
// hardcoded ClientSideConnection.handle because it returns MethodNotFound
// for our extension method.
func newDispatcher(client *streamingClient, logger *zap.Logger) acp.MethodHandler {
	return func(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
		switch method {
		case acp.ClientMethodSessionUpdate:
			var p acp.SessionNotification
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
			}
			if err := client.SessionUpdate(ctx, p); err != nil {
				return nil, toRequestError(err)
			}
			return nil, nil

		case acp.ClientMethodSessionRequestPermission:
			var p acp.RequestPermissionRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
			}
			resp, err := client.RequestPermission(ctx, p)
			if err != nil {
				return nil, toRequestError(err)
			}
			return resp, nil

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
func handleJaegerToolCall(params json.RawMessage, logger *zap.Logger) (any, *acp.RequestError) {
	var req extToolCallRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
	}
	logger.Info("contextual tool call received from sidecar (AG-UI relay pending)",
		zap.String("session_id", req.SessionID),
		zap.String("tool", req.Name),
		zap.ByteString("args", req.Args),
	)
	return extToolCallResponse{
		Result: nil,
		Note:   "tool logged, AG-UI relay not yet wired",
	}, nil
}

// toRequestError preserves an existing *acp.RequestError (so handlers can
// return precise error codes) and wraps any other error as InternalError.
func toRequestError(err error) *acp.RequestError {
	var re *acp.RequestError
	if errors.As(err, &re) {
		return re
	}
	return acp.NewInternalError(map[string]any{"error": err.Error()})
}
