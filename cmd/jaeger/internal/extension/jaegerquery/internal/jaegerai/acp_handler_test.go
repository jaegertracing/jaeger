// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// freshACPHandler returns a dispatcher fixture with an empty store. Tests
// that exercise contextual tool dispatches register the tool against
// fixture.store before invoking fixture.d.
func freshACPHandler(t *testing.T) dispatcherFixture {
	t.Helper()
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	store := NewContextualToolsStore()
	core, logs := observer.New(zap.InfoLevel)
	return dispatcherFixture{
		d:     newACPHandler(client, store, zap.New(core)),
		store: store,
		rr:    rr,
		logs:  logs,
	}
}

func TestACPHandlerSessionUpdateForwardsToStreamingClient(t *testing.T) {
	f := freshACPHandler(t)
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

func TestACPHandlerSessionUpdateInvalidParamsErrors(t *testing.T) {
	d := freshACPHandler(t).d

	_, reqErr := d(t.Context(), acp.ClientMethodSessionUpdate, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code, "invalid JSON should yield InvalidParams")
}

func TestACPHandlerToolCallStripsUIPrefixAndLogsBoth(t *testing.T) {
	f := freshACPHandler(t)
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

func TestACPHandlerToolCallWarnsWhenPrefixMissing(t *testing.T) {
	f := freshACPHandler(t)
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

func TestACPHandlerToolCallRejectsUnknownTool(t *testing.T) {
	// The store has a tool, but the sidecar dispatches a different one.
	// The dispatcher must reject so a misbehaving sidecar / LLM can't
	// invoke a tool the frontend never declared.
	f := freshACPHandler(t)
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

func TestACPHandlerToolCallSkipsNonObjectEntriesInSnapshot(t *testing.T) {
	// The store keeps tool snapshots as raw JSON and unmarshals them into
	// any on lookup; a stored value that decodes to anything other than an
	// object (string, number, etc.) cannot carry a "name" field and must
	// be skipped silently. Pairing one bogus entry with one valid entry
	// proves the loop continues past the bogus one and still matches.
	f := freshACPHandler(t)
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

func TestACPHandlerToolCallRejectsUnknownSession(t *testing.T) {
	// No SetForSession was called for this session id (e.g. the
	// chat handler's defer Delete already ran, or this dispatch landed
	// against a session that never registered any contextual tools).
	d := freshACPHandler(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-stale",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "dispatch to a session with no registered tools must be rejected")
	assert.Equal(t, -32602, reqErr.Code, "unknown session should yield InvalidParams")
}

func TestACPHandlerToolCallRejectsWhenStoreIsNil(t *testing.T) {
	// nil store guards against a misconfigured handler — every contextual
	// dispatch becomes a hard rejection rather than a silent ack.
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	d := newACPHandler(client, nil, zap.NewNop())

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "nil store should reject contextual dispatches as not-registered")
	assert.Equal(t, -32602, reqErr.Code)
}

func TestACPHandlerToolCallInvalidParamsErrors(t *testing.T) {
	d := freshACPHandler(t).d

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestACPHandlerToolCallRejectsEmptySessionID(t *testing.T) {
	d := freshACPHandler(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "",
		Name:      UIToolPrefix + "render_chart",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "empty sessionId must surface as a hard error, not a silent ack success")
	assert.Equal(t, -32602, reqErr.Code, "missing required field should yield InvalidParams")
}

func TestACPHandlerToolCallRejectsEmptyName(t *testing.T) {
	d := freshACPHandler(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      "",
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "empty tool name must surface as a hard error, not a silent ack success")
	assert.Equal(t, -32602, reqErr.Code, "missing required field should yield InvalidParams")
}

func TestACPHandlerToolCallRejectsPrefixOnlyName(t *testing.T) {
	// A name that is exactly UIToolPrefix would strip to "" — we must reject
	// it instead of accepting a tool call with no actual name.
	d := freshACPHandler(t).d

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix,
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.NotNil(t, reqErr, "prefix-only name must not be accepted as a successful tool call")
	assert.Equal(t, -32602, reqErr.Code, "prefix-only name should yield InvalidParams")
}

func TestACPHandlerUnknownMethodReturnsMethodNotFound(t *testing.T) {
	d := freshACPHandler(t).d

	_, reqErr := d(t.Context(), "_meta/unknown/something", json.RawMessage(`{}`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code, "unknown method should yield MethodNotFound")
}

func TestACPHandlerRequestPermissionDelegatesToStreamingClient(t *testing.T) {
	d := freshACPHandler(t).d

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

func TestACPHandlerRequestPermissionInvalidParamsErrors(t *testing.T) {
	d := freshACPHandler(t).d

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

func TestACPHandlerToolCallLogsDeprecationWarning(t *testing.T) {
	f := freshACPHandler(t)
	d, store, logs := f.d, f.store, f.logs
	store.SetForSession("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})

	params, err := json.Marshal(extToolCallRequest{
		SessionID: "sess-abc",
		Name:      UIToolPrefix + "render_chart",
		Args:      json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_, reqErr := d(t.Context(), ExtMethodJaegerToolCall, params)
	require.Nil(t, reqErr)

	// Verify deprecation warning was logged
	foundDeprecation := false
	for _, entry := range logs.All() {
		if entry.Level == zap.WarnLevel && entry.Message == "invoked deprecated ACP extension method _meta/jaegertracing.io/tools/call; migrate to MCP tools/call" {
			foundDeprecation = true
			break
		}
	}
	assert.True(t, foundDeprecation, "deprecation warning must be logged when invoking ExtMethodJaegerToolCall")
}

type mockUpstreamCaller struct {
	listToolsFunc func(ctx context.Context) (*mcp.ListToolsResult, error)
	callToolFunc  func(ctx context.Context, name string, args any) (*mcp.CallToolResult, error)
	closeFunc     func() error
}

func (m *mockUpstreamCaller) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	if m.listToolsFunc != nil {
		return m.listToolsFunc(ctx)
	}
	return &mcp.ListToolsResult{}, nil
}

func (m *mockUpstreamCaller) CallTool(ctx context.Context, name string, args any) (*mcp.CallToolResult, error) {
	if m.callToolFunc != nil {
		return m.callToolFunc(ctx, name, args)
	}
	return &mcp.CallToolResult{}, nil
}

func (m *mockUpstreamCaller) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type mcpACPFixture struct {
	d       acp.MethodHandler
	turns   *turnRegistry
	routeID string
	closer  func()
	rr      *httptest.ResponseRecorder
}

func setupMCPACPHandler(t *testing.T, upstream UpstreamCaller, tools ...json.RawMessage) mcpACPFixture {
	t.Helper()
	rr := httptest.NewRecorder()
	client := newStreamingClient(t.Context(), rr, "thread-test", "run-test")
	store := NewContextualToolsStore()
	turns := newTurnRegistry()
	routeID, closer := turns.register(client, tools)

	handler := newACPHandler(
		client,
		store,
		zap.NewNop(),
		WithACPHandlerTurns(turns),
		WithACPHandlerUpstream(upstream),
	)
	return mcpACPFixture{
		d:       handler,
		turns:   turns,
		routeID: routeID,
		closer:  closer,
		rr:      rr,
	}
}

func TestACPHandlerMcpConnectSuccess(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	params, err := json.Marshal(acp.UnstableConnectMcpRequest{
		AcpId: acp.UnstableMcpServerAcpId(f.routeID),
	})
	require.NoError(t, err)

	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpConnect, params)
	require.Nil(t, reqErr)
	resp, ok := res.(acp.UnstableConnectMcpResponse)
	require.True(t, ok)
	assert.NotEmpty(t, resp.ConnectionId)
}

func TestACPHandlerMcpConnectErrors(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	// Malformed JSON
	_, reqErr := f.d(t.Context(), acp.ClientMethodMcpConnect, json.RawMessage(`{invalid-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)

	// Empty AcpId
	params, err := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: ""})
	require.NoError(t, err)
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpConnect, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)

	// Unknown AcpId
	params, err = json.Marshal(acp.UnstableConnectMcpRequest{AcpId: "non-existent-turn"})
	require.NoError(t, err)
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpConnect, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestACPHandlerMcpDisconnectSuccess(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	require.Nil(t, reqErr)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	disconnectParams, _ := json.Marshal(acp.UnstableDisconnectMcpRequest{ConnectionId: connID})
	res, reqErr = f.d(t.Context(), acp.ClientMethodMcpDisconnect, disconnectParams)
	require.Nil(t, reqErr)
	_, ok := res.(acp.UnstableDisconnectMcpResponse)
	require.True(t, ok)

	// Disconnecting again removes nothing and succeeds cleanly
	res, reqErr = f.d(t.Context(), acp.ClientMethodMcpDisconnect, disconnectParams)
	require.Nil(t, reqErr)
	_, ok = res.(acp.UnstableDisconnectMcpResponse)
	require.True(t, ok)
}

func TestACPHandlerMcpDisconnectErrors(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	// Malformed JSON
	_, reqErr := f.d(t.Context(), acp.ClientMethodMcpDisconnect, json.RawMessage(`{not-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)

	// Empty ConnectionId
	params, _ := json.Marshal(acp.UnstableDisconnectMcpRequest{ConnectionId: ""})
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpDisconnect, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestACPHandlerMcpMessageErrors(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	// Malformed JSON
	_, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, json.RawMessage(`{bad-json`))
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)

	// Empty ConnectionId
	params, _ := json.Marshal(acp.UnstableMessageMcpRequest{ConnectionId: ""})
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpMessage, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)

	// Unknown ConnectionId
	params, _ = json.Marshal(acp.UnstableMessageMcpRequest{ConnectionId: "unknown-conn", Method: "ping"})
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpMessage, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)

	// Turn closed/expired
	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	// Unregister turn
	f.closer()
	params, _ = json.Marshal(acp.UnstableMessageMcpRequest{ConnectionId: connID, Method: "ping"})
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpMessage, params)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}

func TestACPHandlerMcpMessageInitializeAndPing(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	// initialize
	initParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "initialize",
	})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, initParams)
	require.Nil(t, reqErr)
	initMap, ok := res.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2024-11-05", initMap["protocolVersion"])
	caps, ok := initMap["capabilities"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, caps, "tools")

	// ping
	pingParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "ping",
	})
	res, reqErr = f.d(t.Context(), acp.ClientMethodMcpMessage, pingParams)
	require.Nil(t, reqErr)
	assert.Equal(t, map[string]any{}, res)

	// unknown method
	unknownParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "unknown/method",
	})
	_, reqErr = f.d(t.Context(), acp.ClientMethodMcpMessage, unknownParams)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32601, reqErr.Code)
}

func TestACPHandlerMcpMessageToolsList(t *testing.T) {
	mockUpstream := &mockUpstreamCaller{
		listToolsFunc: func(_ context.Context) (*mcp.ListToolsResult, error) {
			return &mcp.ListToolsResult{
				Tools: []*mcp.Tool{
					{Name: "get_services", Description: "telemetry tool"},
				},
			}, nil
		},
	}

	f := setupMCPACPHandler(t, mockUpstream,
		json.RawMessage(`{"name":"render_chart","description":"ui chart tool"}`),
	)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	listParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/list",
	})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, listParams)
	require.Nil(t, reqErr)
	toolsResult, ok := res.(*mcp.ListToolsResult)
	require.True(t, ok)
	require.Len(t, toolsResult.Tools, 2)

	// Telemetry tool retains name, UI tool gets ui_ prefix
	names := []string{toolsResult.Tools[0].Name, toolsResult.Tools[1].Name}
	assert.Contains(t, names, "get_services")
	assert.Contains(t, names, "ui_render_chart")
}

func TestACPHandlerMcpMessageToolsListUpstreamErrorHandledGracefully(t *testing.T) {
	mockUpstream := &mockUpstreamCaller{
		listToolsFunc: func(_ context.Context) (*mcp.ListToolsResult, error) {
			return nil, errors.New("upstream connection failed")
		},
	}

	f := setupMCPACPHandler(t, mockUpstream,
		json.RawMessage(`{"name":"render_chart"}`),
	)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	listParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/list",
	})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, listParams)
	require.Nil(t, reqErr)
	toolsResult, ok := res.(*mcp.ListToolsResult)
	require.True(t, ok)
	// Still returns UI tools despite upstream failure
	require.Len(t, toolsResult.Tools, 1)
	assert.Equal(t, "ui_render_chart", toolsResult.Tools[0].Name)
}

func TestACPHandlerMcpMessageToolsCallUITool(t *testing.T) {
	f := setupMCPACPHandler(t, nil,
		json.RawMessage(`{"name":"render_chart"}`),
	)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	// Call using prefixed name "ui_render_chart"
	callParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/call",
		Params: map[string]any{
			"name":      "ui_render_chart",
			"arguments": map[string]any{"type": "flame"},
		},
	})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, callParams)
	require.Nil(t, reqErr)
	callResult, ok := res.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.False(t, callResult.IsError)
	assert.Contains(t, f.rr.Body.String(), "render_chart")

	// Call using unprefixed name "render_chart"
	callParamsUnprefixed, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/call",
		Params: map[string]any{
			"name":      "render_chart",
			"arguments": map[string]any{"type": "timeline"},
		},
	})
	res, reqErr = f.d(t.Context(), acp.ClientMethodMcpMessage, callParamsUnprefixed)
	require.Nil(t, reqErr)
	callResult, ok = res.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.False(t, callResult.IsError)
}

func TestACPHandlerMcpMessageToolsCallTelemetryTool(t *testing.T) {
	calledWithArgs := false
	mockUpstream := &mockUpstreamCaller{
		callToolFunc: func(_ context.Context, name string, args any) (*mcp.CallToolResult, error) {
			assert.Equal(t, "get_services", name)
			if args != nil {
				calledWithArgs = true
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `["order-service", "customer-service"]`}},
			}, nil
		},
	}

	f := setupMCPACPHandler(t, mockUpstream)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	callParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/call",
		Params: map[string]any{
			"name":      "get_services",
			"arguments": map[string]any{"limit": 10},
		},
	})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, callParams)
	require.Nil(t, reqErr)
	callResult, ok := res.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.False(t, callResult.IsError)
	assert.True(t, calledWithArgs)

	// Stream should receive tool call start & end events
	assert.Contains(t, f.rr.Body.String(), "get_services")
}

func TestACPHandlerMcpMessageToolsCallTelemetryToolNoUpstream(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	callParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/call",
		Params: map[string]any{
			"name": "get_services",
		},
	})
	res, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, callParams)
	require.Nil(t, reqErr)
	callResult, ok := res.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.True(t, callResult.IsError)
	assert.Contains(t, callResult.Content[0].(*mcp.TextContent).Text, "not found or upstream unavailable")
}

func TestACPHandlerMcpMessageToolsCallUpstreamError(t *testing.T) {
	mockUpstream := &mockUpstreamCaller{
		callToolFunc: func(_ context.Context, _ string, _ any) (*mcp.CallToolResult, error) {
			return nil, errors.New("upstream rpc failed")
		},
	}

	f := setupMCPACPHandler(t, mockUpstream)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	callParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/call",
		Params: map[string]any{
			"name": "get_services",
		},
	})
	_, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, callParams)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32603, reqErr.Code)
}

func TestACPHandlerMcpMessageToolsCallMissingName(t *testing.T) {
	f := setupMCPACPHandler(t, nil)

	connectParams, _ := json.Marshal(acp.UnstableConnectMcpRequest{AcpId: acp.UnstableMcpServerAcpId(f.routeID)})
	res, _ := f.d(t.Context(), acp.ClientMethodMcpConnect, connectParams)
	connID := res.(acp.UnstableConnectMcpResponse).ConnectionId

	callParams, _ := json.Marshal(acp.UnstableMessageMcpRequest{
		ConnectionId: connID,
		Method:       "tools/call",
		Params: map[string]any{
			"arguments": map[string]any{},
		},
	})
	_, reqErr := f.d(t.Context(), acp.ClientMethodMcpMessage, callParams)
	require.NotNil(t, reqErr)
	assert.Equal(t, -32602, reqErr.Code)
}
