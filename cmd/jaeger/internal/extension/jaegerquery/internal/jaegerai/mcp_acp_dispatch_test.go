// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newACPProxyHarness wires the same in-memory fixture the HTTP tests
// use, minus the wrapped streamable HTTP handler: tests here exercise
// the gateway-side mcp/connect, mcp/message, mcp/disconnect handlers
// directly, so they don't need an MCP HTTP client.
//
// The returned proxy already has a uuid → sessionID mapping registered
// for `acpID = "jaeger-mcp"` so individual test cases can call
// HandleConnect without re-doing the chat-handler setup. Tests that
// want to assert resolve-failure behaviour pass a different acpId.
func newACPProxyHarness(t *testing.T, sessionID string, uiTools []json.RawMessage, upstreamURL string) (*MCPProxy, *streamingClient, *httptest.ResponseRecorder) {
	t.Helper()
	ctxTools := NewContextualToolsStore()
	if len(uiTools) > 0 {
		ctxTools.SetForSession(sessionID, uiTools)
	}
	streams := NewSessionStreams()

	rec := httptest.NewRecorder()
	sc := newStreamingClient(t.Context(), rec, "thread-test", "run-test")
	sc.startRun()
	streams.Set(sessionID, sc)

	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), nil, "", ctxTools, streams, upstreamURL)
	proxy.RegisterUUIDForSession("jaeger-mcp", sessionID)
	t.Cleanup(func() { _ = proxy.Close() })
	return proxy, sc, rec
}

func TestAnnounceMCPServersGatedByAgentCapability(t *testing.T) {
	// Capability handshake: gateway must NOT advertise type:"acp" when
	// the agent didn't say it supports it; conversely it must always
	// advertise type:"http" when the agent supports HTTP (today's
	// default for every shipping ACP agent). A nil proxy or empty
	// UUID short-circuits to an empty list regardless of caps.
	proxy, _, _ := newACPProxyHarness(t, "sess-cap", nil, "")

	httpOnly := acp.InitializeResponse{
		AgentCapabilities: acp.AgentCapabilities{
			McpCapabilities: acp.McpCapabilities{Http: true},
		},
	}
	httpAndAcp := acp.InitializeResponse{
		AgentCapabilities: acp.AgentCapabilities{
			McpCapabilities: acp.McpCapabilities{Http: true, Acp: true},
		},
	}
	none := acp.InitializeResponse{}

	t.Run("http only → one entry with type http", func(t *testing.T) {
		got := announceMCPServers(httpOnly, proxy, "u-1")
		require.Len(t, got, 1)
		require.NotNil(t, got[0].Http)
		assert.Contains(t, got[0].Http.Url, "/u-1/", "URL must embed the uuid")
		assert.Equal(t, "jaeger", got[0].Http.Name)
	})
	t.Run("http + acp → two entries", func(t *testing.T) {
		got := announceMCPServers(httpAndAcp, proxy, "u-2")
		require.Len(t, got, 2)
		require.NotNil(t, got[0].Http)
		require.NotNil(t, got[1].Acp)
		assert.Equal(t, "u-2", string(got[1].Acp.Id),
			"the announced acpId must be the uuid so HandleConnect can resolve through the same map")
	})
	t.Run("no capabilities → empty list", func(t *testing.T) {
		got := announceMCPServers(none, proxy, "u-3")
		require.NotNil(t, got)
		assert.Empty(t, got)
	})
	t.Run("nil proxy → empty list even with caps", func(t *testing.T) {
		got := announceMCPServers(httpAndAcp, nil, "u-4")
		require.NotNil(t, got)
		assert.Empty(t, got)
	})
	t.Run("empty uuid → empty list even with caps", func(t *testing.T) {
		got := announceMCPServers(httpAndAcp, proxy, "")
		require.NotNil(t, got)
		assert.Empty(t, got)
	})
	t.Run("http url embeds proxy.basePath", func(t *testing.T) {
		// Operators run jaeger-query behind a non-empty basePath
		// (e.g. /jaeger); the announced URL must include it or the
		// agent dials a 404 against the bare /api/ai/mcp/ path that
		// the proxy never mounted at.
		ctxTools := NewContextualToolsStore()
		streams := NewSessionStreams()
		proxyWithBase := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), nil, "/jaeger", ctxTools, streams, "")
		t.Cleanup(func() { _ = proxyWithBase.Close() })

		got := announceMCPServers(httpOnly, proxyWithBase, "u-base")
		require.Len(t, got, 1)
		require.NotNil(t, got[0].Http)
		assert.Equal(t, "http://127.0.0.1:16686/jaeger/api/ai/mcp/u-base/", got[0].Http.Url)
	})
}

func TestUUIDMapRegisterAndResolve(t *testing.T) {
	// The uuid→session map is the single source of truth both transports
	// resolve against. Register, resolve, unregister, and verify empty
	// inputs are no-ops.
	proxy, _, _ := newACPProxyHarness(t, "sess-map", nil, "")
	uuid := NewSessionUUID()
	assert.Empty(t, proxy.resolveSessionFromUUID(uuid),
		"unregistered uuid must resolve to empty string")

	proxy.RegisterUUIDForSession(uuid, "sess-X")
	assert.Equal(t, "sess-X", proxy.resolveSessionFromUUID(uuid))

	proxy.UnregisterUUID(uuid)
	assert.Empty(t, proxy.resolveSessionFromUUID(uuid),
		"unregister must drop the mapping cleanly")

	// Empty inputs are no-ops — the chat handler can pass pre-init
	// state through without guarding every call site.
	proxy.RegisterUUIDForSession("", "sess-X")
	proxy.RegisterUUIDForSession(uuid, "")
	proxy.UnregisterUUID("")
	assert.Empty(t, proxy.resolveSessionFromUUID(""),
		"empty uuid lookup must not surface a stale entry")
}

func TestMCPACPConnectionsLifecycle(t *testing.T) {
	conns := newMCPACPConnections()
	conn := &mcpACPConnection{connectionID: "c1", acpID: "a1", sessionID: "s1"}

	assert.Nil(t, conns.get("c1"), "unknown id returns nil")
	conns.set(conn)
	assert.Same(t, conn, conns.get("c1"))
	conns.delete("c1")
	assert.Nil(t, conns.get("c1"))
	conns.delete("c1") // idempotent
}

func TestHandleConnectAllocatesUniqueConnectionIDs(t *testing.T) {
	// Two mcp/connect calls with the same acpId yield two distinct
	// connectionIds — the SDK allows multiple connections per
	// announced server, and a future agent might open more than one
	// for retry or multiplexing.
	proxy, _, _ := newACPProxyHarness(t, "sess-conn", nil, "")
	req := acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"}

	resp1, err := proxy.HandleConnect(t.Context(), req)
	require.NoError(t, err)
	resp2, err := proxy.HandleConnect(t.Context(), req)
	require.NoError(t, err)
	assert.NotEqual(t, resp1.ConnectionId, resp2.ConnectionId,
		"connection ids must be unique even when acpId is reused")
	assert.NotEmpty(t, string(resp1.ConnectionId))
}

func TestHandleConnectRejectsEmptyAcpID(t *testing.T) {
	proxy, _, _ := newACPProxyHarness(t, "sess-empty", nil, "")
	_, err := proxy.HandleConnect(t.Context(), acp.UnstableConnectMcpRequest{AcpId: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acpId is required")
}

func TestHandleConnectRejectsUnknownAcpID(t *testing.T) {
	// An acpId that doesn't appear in the uuid→session map is the
	// already-expired / never-registered / malicious case. Reject with a
	// clear message so the agent can fall back to HTTP or retry.
	proxy, _, _ := newACPProxyHarness(t, "sess-unknown", nil, "")
	_, err := proxy.HandleConnect(t.Context(), acp.UnstableConnectMcpRequest{AcpId: "stale-uuid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown acpId")
}

func TestHandleDisconnectIsIdempotent(t *testing.T) {
	proxy, _, _ := newACPProxyHarness(t, "sess-dc", nil, "")
	resp, err := proxy.HandleDisconnect(t.Context(), acp.UnstableDisconnectMcpRequest{ConnectionId: "never-existed"})
	require.NoError(t, err)
	assert.Equal(t, acp.UnstableDisconnectMcpResponse{}, resp)
}

func TestHandleMessageReturnsToolsListForKnownSession(t *testing.T) {
	// End-to-end: open a connection, then send a tools/list as an inner
	// MCP message. The response should be a ListToolsResult whose UI
	// tool names are UIToolPrefix-namespaced on the wire — that's the
	// collision-safe shape the agent sees.
	highlight := sampleUITool(t, "highlight_span", "Highlight a span",
		map[string]any{"type": "object"})
	proxy, _, _ := newACPProxyHarness(t, "sess-list", []json.RawMessage{highlight}, "")

	connectResp, err := proxy.HandleConnect(t.Context(), acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
	require.NoError(t, err)

	rawResp, err := proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: connectResp.ConnectionId,
		Method:       "tools/list",
	})
	require.NoError(t, err)

	listed, ok := rawResp.(*mcp.ListToolsResult)
	require.True(t, ok, "tools/list response must be *mcp.ListToolsResult; got %T", rawResp)
	require.Len(t, listed.Tools, 1)
	assert.Equal(t, "ui_highlight_span", listed.Tools[0].Name,
		"UI tool names must carry UIToolPrefix on the wire")
}

func TestHandleMessageRoutesToolCallToSSEForUITool(t *testing.T) {
	// The whole point of MCP-over-ACP: an inner tools/call for a
	// (prefixed) UI tool must strip the prefix, find the unprefixed
	// entry in ctxTools, and surface TOOL_CALL_START/ARGS/END on the
	// SSE stream — same contract as the HTTP transport.
	highlight := sampleUITool(t, "highlight_span", "Highlight a span",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{"spanId": map[string]any{"type": "string"}},
		})
	proxy, _, rec := newACPProxyHarness(t, "sess-call", []json.RawMessage{highlight}, "")

	connectResp, err := proxy.HandleConnect(t.Context(), acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
	require.NoError(t, err)

	rawResp, err := proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: connectResp.ConnectionId,
		Method:       "tools/call",
		Params: map[string]any{
			"name":      "ui_highlight_span",
			"arguments": map[string]any{"spanId": "abc123"},
		},
	})
	require.NoError(t, err)

	result, ok := rawResp.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.False(t, result.IsError)

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	require.Contains(t, types, "TOOL_CALL_START")
	require.Contains(t, types, "TOOL_CALL_ARGS")
	require.Contains(t, types, "TOOL_CALL_END")
	assert.NotContains(t, types, "TOOL_CALL_RESULT",
		"UI dispatch must not emit TOOL_CALL_RESULT — browser is the executor")
}

func TestHandleMessageReturnsErrorOnUnknownConnection(t *testing.T) {
	proxy, _, _ := newACPProxyHarness(t, "sess-stale", nil, "")
	_, err := proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: "ghost-connection",
		Method:       "tools/list",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown connectionId")
}

func TestHandleMessageReturnsInitializeShape(t *testing.T) {
	proxy, _, _ := newACPProxyHarness(t, "sess-init", nil, "")
	connectResp, err := proxy.HandleConnect(t.Context(), acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
	require.NoError(t, err)

	rawResp, err := proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: connectResp.ConnectionId,
		Method:       "initialize",
	})
	require.NoError(t, err)

	init, ok := rawResp.(*mcp.InitializeResult)
	require.True(t, ok)
	assert.Equal(t, mcpOverACPProtocolVersion, init.ProtocolVersion)
	require.NotNil(t, init.Capabilities)
	require.NotNil(t, init.Capabilities.Tools,
		"agents that infer tool capability from initialize need .tools to be present (even as an empty object)")
	require.NotNil(t, init.ServerInfo)
	assert.Equal(t, mcpServerName, init.ServerInfo.Name)
}

func TestHandleMessageRejectsUnknownMethod(t *testing.T) {
	proxy, _, _ := newACPProxyHarness(t, "sess-unk", nil, "")
	connectResp, err := proxy.HandleConnect(t.Context(), acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
	require.NoError(t, err)

	_, err = proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: connectResp.ConnectionId,
		Method:       "resources/list",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestListToolsForSessionMergesUIAndUpstream(t *testing.T) {
	upstream := startFakeUpstreamMCP(t, "get_services")
	highlight := sampleUITool(t, "highlight_span", "Highlight a span",
		map[string]any{"type": "object"})
	proxy, _, _ := newACPProxyHarness(t, "sess-merge", []json.RawMessage{highlight}, upstream)

	tools := proxy.listToolsForSession("sess-merge")
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"ui_highlight_span", "get_services"}, names,
		"UI tools carry the ui_ prefix on the wire; upstream tools never do")
}

func TestApplyUIToolPrefixIsIdempotent(t *testing.T) {
	// Frontends that pre-prefix their names (or paths that round-trip
	// through the wire and back) must not end up with double prefixes.
	assert.Equal(t, "ui_highlight_span", applyUIToolPrefix("highlight_span"))
	assert.Equal(t, "ui_highlight_span", applyUIToolPrefix("ui_highlight_span"))
}

func TestStripUIToolPrefixOKDistinguishesUIFromTelemetry(t *testing.T) {
	// callToolForSession uses the bool to skip the UI lookup path
	// entirely when the name lacks the prefix — without it, telemetry
	// tool calls would spuriously consult ctxTools.
	stripped, ok := stripUIToolPrefixOK("ui_highlight_span")
	assert.True(t, ok)
	assert.Equal(t, "highlight_span", stripped)

	stripped, ok = stripUIToolPrefixOK("get_services")
	assert.False(t, ok, "names without the prefix are telemetry tools, not UI tools")
	assert.Equal(t, "get_services", stripped)

	// Exactly "ui_" strips to "" which isn't a valid tool name — fall
	// through to telemetry semantics (the lookup will then fail at the
	// upstream-tools loop with a clean "unknown tool" error).
	stripped, ok = stripUIToolPrefixOK("ui_")
	assert.False(t, ok)
	assert.Equal(t, "ui_", stripped)
}
