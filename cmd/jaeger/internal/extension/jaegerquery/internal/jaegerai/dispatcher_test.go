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
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// freshDispatcher returns a dispatcher backed by a streamingClient writing
// to an httptest.ResponseRecorder, so SessionUpdate-driven writes show up
// in rr.Body for assertion.
func freshDispatcher(t *testing.T) (acp.MethodHandler, *httptest.ResponseRecorder, *observer.ObservedLogs) {
	t.Helper()
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr)
	core, logs := observer.New(zap.InfoLevel)
	return newDispatcher(client, zap.New(core)), rr, logs
}

func TestDispatcherSessionUpdateForwardsToStreamingClient(t *testing.T) {
	d, rr, _ := freshDispatcher(t)

	// Marshal a SessionNotification carrying an agent message chunk; the
	// dispatcher should hand it to streamingClient and the text should
	// land in the response writer.
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
	d, _, _ := freshDispatcher(t)

	_, reqErr := d(t.Context(), acp.ClientMethodSessionUpdate, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "invalid JSON should yield InvalidParams")
}

func TestDispatcherToolCallStripsUIPrefixAndLogsBoth(t *testing.T) {
	d, _, logs := freshDispatcher(t)

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix + "render_chart",
		Args:      json.RawMessage(`{"kind":"flame"}`),
	})
	require.NoError(t, err)

	result, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.Nil(t, reqErr)

	resp, ok := result.(extToolCallResponse)
	require.True(t, ok, "expected extToolCallResponse, got %T", result)
	assert.Nil(t, resp.Result, "PR1 placeholder must be a null result")
	assert.False(t, resp.IsError, "PR1 placeholder must not flag the call as an error")
	assert.Contains(t, resp.Note, "AG-UI relay not yet wired")

	// The placeholder log must show the stripped name (what the AG-UI
	// client sees) plus the original prefixed name (what Gemini called).
	entries := logs.FilterMessage("contextual tool call received from sidecar (AG-UI relay pending)").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, "sess-abc", fields["session_id"])
	assert.Equal(t, "render_chart", fields["tool"], "stripped name should be logged under 'tool'")
	assert.Equal(t, UIToolPrefix+"render_chart", fields["prefixed_tool"])

	// Stripping happens silently — no warning when the prefix is present.
	require.Empty(t, logs.FilterMessage("contextual tool name missing UI prefix; passing through unchanged").All())
}

func TestDispatcherToolCallWarnsWhenPrefixMissing(t *testing.T) {
	d, _, logs := freshDispatcher(t)

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      "render_chart", // no UIToolPrefix → defensive pass-through + warning
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.Nil(t, reqErr, "missing prefix is a warning, not a hard error")

	warnings := logs.FilterMessage("contextual tool name missing UI prefix; passing through unchanged").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "render_chart", warnings[0].ContextMap()["tool"])
}

func TestDispatcherToolCallInvalidParamsErrors(t *testing.T) {
	d, _, _ := freshDispatcher(t)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestDispatcherToolCallRejectsEmptySessionID(t *testing.T) {
	d, _, _ := freshDispatcher(t)

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "empty sessionId must surface as a hard error, not a silent placeholder success")
	assert.Equal(t, -32602, reqErr.Code, "missing required field should yield InvalidParams")
}

func TestDispatcherToolCallRejectsEmptyName(t *testing.T) {
	d, _, _ := freshDispatcher(t)

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      "",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "empty tool name must surface as a hard error, not a silent placeholder success")
	assert.Equal(t, -32602, reqErr.Code, "missing required field should yield InvalidParams")
}

func TestDispatcherUnknownMethodReturnsMethodNotFound(t *testing.T) {
	d, _, _ := freshDispatcher(t)

	_, reqErr := d(t.Context(), "_meta/unknown/something", json.RawMessage(`{}`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code, "unknown method should yield MethodNotFound")
}

func TestDispatcherRequestPermissionDelegatesToStreamingClient(t *testing.T) {
	d, _, _ := freshDispatcher(t)

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
	d, _, _ := freshDispatcher(t)

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
