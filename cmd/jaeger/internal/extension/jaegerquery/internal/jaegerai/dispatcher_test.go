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

// dispatcherFixture bundles everything a dispatcher test usually wants:
// the dispatcher itself, the store it consults for ext_method calls, the
// recorder its streaming client writes into, and the captured log buffer.
type dispatcherFixture struct {
	d     acp.MethodHandler
	store *ContextualToolsStore
	rr    *httptest.ResponseRecorder
	logs  *observer.ObservedLogs
}

// freshDispatcher returns a dispatcher fixture with an empty store. Tests
// that exercise contextual tool dispatches register the tool against
// fixture.store before invoking fixture.d.
func freshDispatcher(t *testing.T) dispatcherFixture {
	t.Helper()
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	store := NewContextualToolsStore()
	core, logs := observer.New(zap.InfoLevel)
	return dispatcherFixture{
		d:     newDispatcher(client, store, zap.New(core)),
		store: store,
		rr:    rr,
		logs:  logs,
	}
}

func TestDispatcherSessionUpdateForwardsToStreamingClient(t *testing.T) {
	f := freshDispatcher(t)
	d, rr := f.d, f.rr

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
	d := freshDispatcher(t).d

	_, reqErr := d(t.Context(), acp.ClientMethodSessionUpdate, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "invalid JSON should yield InvalidParams")
}

func TestDispatcherToolCallStripsUIPrefixAndLogsBoth(t *testing.T) {
	f := freshDispatcher(t)
	d, store, logs := f.d, f.store, f.logs
	store.SetForSession("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})

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
	assert.False(t, resp.IsError, "fire-and-forget ack must not flag the call as an error")
	ack, ok := resp.Result.(map[string]any)
	require.True(t, ok, "fire-and-forget ack should carry a map result, got %T", resp.Result)
	assert.Equal(t, true, ack["acknowledged"],
		"contextual tool dispatch must return an acknowledged=true result so Gemini's loop continues")

	// The dispatch log must show the stripped name (what the AG-UI client
	// sees) plus the original prefixed name (what Gemini called).
	entries := logs.FilterMessage("contextual tool call dispatched (fire-and-forget)").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, "sess-abc", fields["session_id"])
	assert.Equal(t, "render_chart", fields["tool"], "stripped name should be logged under 'tool'")
	assert.Equal(t, UIToolPrefix+"render_chart", fields["prefixed_tool"])

	// Stripping happens silently — no warning when the prefix is present.
	require.Empty(t, logs.FilterMessage("contextual tool name missing UI prefix; passing through unchanged").All())
}

func TestDispatcherToolCallWarnsWhenPrefixMissing(t *testing.T) {
	f := freshDispatcher(t)
	d, store, logs := f.d, f.store, f.logs
	store.SetForSession("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})

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

func TestDispatcherToolCallRejectsUnknownTool(t *testing.T) {
	// The store has a tool, but the sidecar dispatches a different one.
	// The dispatcher must reject so a misbehaving sidecar / LLM can't
	// invoke a tool the frontend never declared.
	f := freshDispatcher(t)
	d, store := f.d, f.store
	store.SetForSession("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix + "unknown_tool",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "unknown contextual tool must be rejected, not silently acked")
	assert.Equal(t, -32602, reqErr.Code, "unknown tool should yield InvalidParams")
}

func TestDispatcherToolCallRejectsUnknownSession(t *testing.T) {
	// No SetForSession was called for this session id (e.g. the
	// chat handler's defer Delete already ran, or this dispatch landed
	// against a session that never registered any contextual tools).
	d := freshDispatcher(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-stale",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "dispatch to a session with no registered tools must be rejected")
	assert.Equal(t, -32602, reqErr.Code, "unknown session should yield InvalidParams")
}

func TestDispatcherToolCallRejectsWhenStoreIsNil(t *testing.T) {
	// nil store guards against a misconfigured handler — every contextual
	// dispatch becomes a hard rejection rather than a silent ack.
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	d := newDispatcher(client, nil, zap.NewNop())

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "nil store should reject contextual dispatches as not-registered")
	assert.Equal(t, -32602, reqErr.Code)
}

func TestDispatcherToolCallInvalidParamsErrors(t *testing.T) {
	d := freshDispatcher(t).d

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestDispatcherToolCallRejectsEmptySessionID(t *testing.T) {
	d := freshDispatcher(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "empty sessionId must surface as a hard error, not a silent ack success")
	assert.Equal(t, -32602, reqErr.Code, "missing required field should yield InvalidParams")
}

func TestDispatcherToolCallRejectsEmptyName(t *testing.T) {
	d := freshDispatcher(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      "",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "empty tool name must surface as a hard error, not a silent ack success")
	assert.Equal(t, -32602, reqErr.Code, "missing required field should yield InvalidParams")
}

func TestDispatcherToolCallRejectsPrefixOnlyName(t *testing.T) {
	// A name that is exactly UIToolPrefix would strip to "" — we must reject
	// it instead of accepting a tool call with no actual name.
	d := freshDispatcher(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix,
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "prefix-only name must not be accepted as a successful tool call")
	assert.Equal(t, -32602, reqErr.Code, "prefix-only name should yield InvalidParams")
}

func TestDispatcherUnknownMethodReturnsMethodNotFound(t *testing.T) {
	d := freshDispatcher(t).d

	_, reqErr := d(t.Context(), "_meta/unknown/something", json.RawMessage(`{}`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code, "unknown method should yield MethodNotFound")
}

func TestDispatcherRequestPermissionDelegatesToStreamingClient(t *testing.T) {
	d := freshDispatcher(t).d

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
	d := freshDispatcher(t).d

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
