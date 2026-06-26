// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freshDispatcher returns a dispatcher with no MCP proxy wired in —
// the three mcp/* methods will degrade to MethodNotFound. Tests that
// exercise the MCP-over-ACP path live in mcp_acp_dispatch_test.go where
// a real proxy harness is available.
func freshDispatcher(t *testing.T) (acp.MethodHandler, *httptest.ResponseRecorder) {
	t.Helper()
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	return newDispatcher(client, nil), rr
}

func TestDispatcherSessionUpdateForwardsToStreamingClient(t *testing.T) {
	d, rr := freshDispatcher(t)

	notif := acp.SessionNotification{
		SessionId: "sess-1",
		Update:    acp.UpdateAgentMessageText("hello from agent"),
	}
	params, err := json.Marshal(notif)
	require.NoError(t, err)

	result, reqErr := d(t.Context(), acp.ClientMethodSessionUpdate, params)
	require.Nil(t, reqErr)
	require.Nil(t, result, "session/update is a notification — no result body expected")
	assert.Contains(t, rr.Body.String(), "hello from agent")
}

func TestDispatcherSessionUpdateInvalidParamsErrors(t *testing.T) {
	d, _ := freshDispatcher(t)

	_, reqErr := d(t.Context(), acp.ClientMethodSessionUpdate, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "invalid JSON should yield InvalidParams")
}

func TestDispatcherUnknownMethodReturnsMethodNotFound(t *testing.T) {
	d, _ := freshDispatcher(t)

	_, reqErr := d(t.Context(), "_meta/unknown/something", json.RawMessage(`{}`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code, "unknown method should yield MethodNotFound")
}

func TestDispatcherMcpMethodsReturnMethodNotFoundWithoutProxy(t *testing.T) {
	// A dispatcher built without an MCPProxy advertises no MCP-over-ACP
	// endpoint; each of the three mcp/* methods must respond with
	// MethodNotFound so the agent knows to fall back (or, more often,
	// that it can't use this transport at all).
	d, _ := freshDispatcher(t)

	for _, method := range []string{
		acp.ClientMethodMcpConnect,
		acp.ClientMethodMcpDisconnect,
		acp.ClientMethodMcpMessage,
	} {
		_, reqErr := d(t.Context(), method, json.RawMessage(`{}`))
		require.NotNil(t, reqErr, "method=%s without proxy must error", method)
		assert.Equal(t, -32601, reqErr.Code, "method=%s without proxy should yield MethodNotFound", method)
	}
}

func TestDispatcherRequestPermissionDelegatesToStreamingClient(t *testing.T) {
	d, _ := freshDispatcher(t)

	params, err := json.Marshal(acp.RequestPermissionRequest{
		SessionId: "sess-1",
		Options:   []acp.PermissionOption{},
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: "tc-1",
		},
	})
	require.NoError(t, err)

	result, reqErr := d(t.Context(), acp.ClientMethodSessionRequestPermission, params)
	require.Nil(t, reqErr)
	resp, ok := result.(acp.RequestPermissionResponse)
	require.True(t, ok)
	require.NotNil(t, resp.Outcome.Cancelled,
		"streamingClient denies permissions because the gateway advertises no fs/terminal capability")
}

func TestDispatcherRequestPermissionInvalidParamsErrors(t *testing.T) {
	d, _ := freshDispatcher(t)

	_, reqErr := d(t.Context(), acp.ClientMethodSessionRequestPermission, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "malformed request_permission params should yield InvalidParams")
}

func TestToRequestErrorReturnsNilForNilInput(t *testing.T) {
	// Lets dispatch sites pass `client.X(...)`'s error through unconditionally
	// without a leading `if err != nil` branch in the dispatcher itself.
	assert.Nil(t, toRequestError(nil))
}

func TestToRequestErrorPreservesACPRequestError(t *testing.T) {
	original := acp.NewInvalidParams(map[string]any{"why": "demo"})
	got := toRequestError(original)
	assert.Same(t, original, got, "existing *acp.RequestError must be returned unchanged")
}

func TestToRequestErrorWrapsPlainError(t *testing.T) {
	got := toRequestError(errors.New("boom"))
	require.NotNil(t, got)
	assert.Equal(t, -32603, got.Code, "plain errors should be wrapped as InternalError")
}
