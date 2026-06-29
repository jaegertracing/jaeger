// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync/atomic"
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
		d:     newDispatcher(client, store, nil, nil, zap.New(core)),
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

	// args must NOT appear in the Info record — they may contain user
	// data, PII, or oversized payloads. Only a size field is emitted at
	// Info; the full payload is reserved for Debug. This assertion pins
	// that contract so a future "let's just log it for debugging"
	// refactor can't quietly regress it.
	_, hasArgs := fields["args"]
	assert.False(t, hasArgs,
		"raw args must not appear in the Info-level record — Debug only, to avoid leaking user data")
	assert.EqualValues(t, len(`{"kind":"flame"}`), fields["args_size_bytes"],
		"the Info record should carry the args size for observability without exposing the payload")

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

func TestDispatcherToolCallSkipsNonObjectEntriesInSnapshot(t *testing.T) {
	// The store keeps tool snapshots as raw JSON and unmarshals them into
	// any on lookup; a stored value that decodes to anything other than an
	// object (string, number, etc.) cannot carry a "name" field and must
	// be skipped silently. Pairing one bogus entry with one valid entry
	// proves the loop continues past the bogus one and still matches.
	f := freshDispatcher(t)
	d, store := f.d, f.store
	store.SetForSession("sess-abc", []json.RawMessage{
		json.RawMessage(`"naked-string"`),          // decodes to string, not map
		json.RawMessage(`{"name":"render_chart"}`), // valid object
	})

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	result, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.Nil(t, reqErr,
		"the non-object entry must be skipped, not abort the lookup before reaching the valid entry")
	resp, ok := result.(extToolCallResponse)
	require.True(t, ok)
	assert.False(t, resp.IsError)
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
	d := newDispatcher(client, nil, nil, nil, zap.NewNop())

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

// dispatcherWithProxy mirrors freshDispatcher but wires a real MCPProxy
// and a sessionIDRef pre-populated with sessionID. Used by the mcp/*
// tests that need the dispatcher's proxy branch (lines 130-167) to
// run rather than fall through to MethodNotFound.
//
// upstreamURL is "" so the proxy doesn't try to dial anything during
// construction — these tests only exercise the dispatcher's typed
// case bodies, not the upstream-tools machinery.
func dispatcherWithProxy(t *testing.T, sessionID string) (acp.MethodHandler, *MCPProxy) {
	t.Helper()
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	store := NewContextualToolsStore()
	streams := NewSessionStreams()
	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", store, streams, "")
	t.Cleanup(func() { _ = proxy.Close() })

	var ref atomic.Pointer[string]
	if sessionID != "" {
		s := sessionID
		ref.Store(&s)
	}
	d := newDispatcher(client, store, proxy, &ref, zap.NewNop())
	return d, proxy
}

func TestDispatcherMCPConnectRoutesToProxy(t *testing.T) {
	// Happy path: with a proxy wired in and a session id already known,
	// mcp/connect must reach proxy.HandleConnect and surface its
	// generated connectionId back to the agent.
	d, _ := dispatcherWithProxy(t, "sess-mcp-conn")
	params, err := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp-1"})
	require.NoError(t, err)

	result, reqErr := d(t.Context(), acp.ClientMethodMcpConnect, params)
	require.Nil(t, reqErr)
	resp, ok := result.(acp.UnstableConnectMcpResponse)
	require.True(t, ok, "expected UnstableConnectMcpResponse, got %T", result)
	assert.NotEmpty(t, string(resp.ConnectionId), "HandleConnect must mint a non-empty connectionId")
}

func TestDispatcherMCPConnectReturnsMethodNotFoundWithoutProxy(t *testing.T) {
	// Capability gating: agents that announce mcp.acp=true still hit
	// MethodNotFound when no proxy is wired (e.g. older deployments
	// without the MCP route). Lets agents fall back to the HTTP path
	// rather than retry indefinitely.
	d := freshDispatcher(t).d
	params, err := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp-1"})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), acp.ClientMethodMcpConnect, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code, "mcp/connect with nil proxy must be MethodNotFound")
}

func TestDispatcherMCPConnectInvalidParamsErrors(t *testing.T) {
	d, _ := dispatcherWithProxy(t, "sess-mcp-conn")
	_, reqErr := d(t.Context(), acp.ClientMethodMcpConnect, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "malformed mcp/connect params should yield InvalidParams")
}

func TestDispatcherMCPConnectBeforeSessionReadyErrors(t *testing.T) {
	// session/new hasn't returned yet → sessionIDRef holds nothing.
	// The dispatcher must refuse mcp/connect rather than dispatch with
	// an empty session id, which would cross-contaminate UI-tool
	// snapshots across chat turns.
	d, _ := dispatcherWithProxy(t, "")
	params, err := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp-1"})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), acp.ClientMethodMcpConnect, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "mcp/connect before session/new should yield InvalidParams")
}

func TestDispatcherMCPDisconnectRoutesToProxy(t *testing.T) {
	// Disconnect targets a connection we opened earlier via Connect.
	d, proxy := dispatcherWithProxy(t, "sess-mcp-dc")
	connResp, err := proxy.HandleConnect(t.Context(), "sess-mcp-dc",
		acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp-1"})
	require.NoError(t, err)

	params, err := json.Marshal(acp.UnstableDisconnectMcpRequest{ConnectionId: connResp.ConnectionId})
	require.NoError(t, err)

	result, reqErr := d(t.Context(), acp.ClientMethodMcpDisconnect, params)
	require.Nil(t, reqErr)
	_, ok := result.(acp.UnstableDisconnectMcpResponse)
	require.True(t, ok, "expected UnstableDisconnectMcpResponse, got %T", result)
}

func TestDispatcherMCPDisconnectReturnsMethodNotFoundWithoutProxy(t *testing.T) {
	d := freshDispatcher(t).d
	params, err := json.Marshal(acp.UnstableDisconnectMcpRequest{ConnectionId: "anything"})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), acp.ClientMethodMcpDisconnect, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code)
}

func TestDispatcherMCPDisconnectInvalidParamsErrors(t *testing.T) {
	d, _ := dispatcherWithProxy(t, "sess-mcp-dc")
	_, reqErr := d(t.Context(), acp.ClientMethodMcpDisconnect, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestDispatcherMCPMessageRoutesToProxy(t *testing.T) {
	// mcp/message tunnels an inner MCP method through ACP; the
	// dispatcher hands it to proxy.HandleMessage which then routes by
	// inner method (initialize, tools/list, tools/call). Use
	// initialize because it's the lightest path that returns a typed
	// SDK response.
	d, proxy := dispatcherWithProxy(t, "sess-mcp-msg")
	connResp, err := proxy.HandleConnect(t.Context(), "sess-mcp-msg",
		acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp-1"})
	require.NoError(t, err)

	params, err := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connResp.ConnectionId,
		Method:       "initialize",
	})
	require.NoError(t, err)

	result, reqErr := d(t.Context(), acp.ClientMethodMcpMessage, params)
	require.Nil(t, reqErr)
	require.NotNil(t, result, "initialize must return an InitializeResult")
}

func TestDispatcherMCPMessageReturnsMethodNotFoundWithoutProxy(t *testing.T) {
	d := freshDispatcher(t).d
	params, err := json.Marshal(acp.UnstableMessageMcpRequest{ConnectionId: "x", Method: "initialize"})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), acp.ClientMethodMcpMessage, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code)
}

func TestDispatcherMCPMessageInvalidParamsErrors(t *testing.T) {
	d, _ := dispatcherWithProxy(t, "sess-mcp-msg")
	_, reqErr := d(t.Context(), acp.ClientMethodMcpMessage, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestCurrentSessionIDHandlesNilAndEmpty(t *testing.T) {
	// currentSessionID is the dispatcher's only read of the
	// session-id holder; covering the three explicit branches keeps
	// the "not yet ready" path from regressing into a nil-pointer
	// panic if the holder shape ever changes.
	assert.Empty(t, currentSessionID(nil), "nil holder → empty string")

	var ref atomic.Pointer[string]
	assert.Empty(t, currentSessionID(&ref), "non-nil holder with nil load → empty string")

	s := "sess-x"
	ref.Store(&s)
	assert.Equal(t, "sess-x", currentSessionID(&ref))
}
