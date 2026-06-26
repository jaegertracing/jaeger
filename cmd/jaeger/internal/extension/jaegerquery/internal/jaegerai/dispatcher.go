// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
)

// UIToolPrefix is the namespace the gateway prepends to UI-tool names
// when advertising them on the MCP wire (tools/list). The prefix
// prevents a frontend-supplied tool from shadowing a built-in jaeger_mcp
// telemetry tool with the same name (e.g. "search_traces") in the
// agent's tool catalogue. callToolForSession strips the prefix on the
// way in so the internal ContextualToolsStore — which stores raw
// frontend names — can find the entry.
const UIToolPrefix = "ui_"

// newDispatcher returns an acp.MethodHandler that routes inbound
// JSON-RPC from the sidecar:
//   - session/update → streamingClient.SessionUpdate (translates ACP
//     updates into typed AG-UI events — TEXT_MESSAGE_* for assistant
//     text and TOOL_CALL_* for tool-call lifecycle — and writes them as
//     SSE frames on the chat HTTP response).
//   - session/request_permission → streamingClient.RequestPermission
//     (always denies; we advertise no fs/terminal capability).
//   - mcp/connect, mcp/disconnect, mcp/message → MCPProxy (the
//     MCP-over-ACP transport, see mcp_acp_dispatch.go). Resolves the
//     per-turn UUID the agent received in mcpServers back to the
//     internal session id through the proxy's uuid→session map.
//   - anything else → MethodNotFound.
//
// proxy may be nil for chat handlers that aren't configured with an
// MCP proxy (tests, deployments without the MCP route wired in). The
// three mcp/* methods degrade to MethodNotFound in that case.
func newDispatcher(client *streamingClient, proxy *MCPProxy) acp.MethodHandler {
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

		case acp.ClientMethodMcpConnect:
			if proxy == nil {
				return nil, acp.NewMethodNotFound(method)
			}
			var p acp.UnstableConnectMcpRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			resp, err := proxy.HandleConnect(ctx, p)
			return resp, toRequestError(err)

		case acp.ClientMethodMcpDisconnect:
			if proxy == nil {
				return nil, acp.NewMethodNotFound(method)
			}
			var p acp.UnstableDisconnectMcpRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			resp, err := proxy.HandleDisconnect(ctx, p)
			return resp, toRequestError(err)

		case acp.ClientMethodMcpMessage:
			if proxy == nil {
				return nil, acp.NewMethodNotFound(method)
			}
			var p acp.UnstableMessageMcpRequest
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("cannot unmarshal request: %v", err)})
			}
			resp, err := proxy.HandleMessage(ctx, p)
			return resp, toRequestError(err)

		default:
			return nil, acp.NewMethodNotFound(method)
		}
	}
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
