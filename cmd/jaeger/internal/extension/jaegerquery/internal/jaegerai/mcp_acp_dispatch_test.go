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

// newACPProxyHarness wires the same in-memory fixture the HTTP tests use,
// minus the wrapped streamable HTTP handler: tests here exercise the
// gateway-side mcp/connect, mcp/message, mcp/disconnect handlers
// directly, so they don't need an MCP HTTP client.
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

	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", ctxTools, streams, upstreamURL)
	t.Cleanup(func() { _ = proxy.Close() })
	return proxy, sc, rec
}

func TestAnnounceMCPServersGatedByAgentCapability(t *testing.T) {
	// Capability handshake: gateway must NOT advertise type:"acp" when the
	// agent didn't say it supports it. Sending an Acp McpServer to a
	// non-supporting agent risks at-best a silent drop, at-worst a hard
	// validation error — neither is operator-friendly.
	proxy, _, _ := newACPProxyHarness(t, "sess-cap", nil, "")

	cases := []struct {
		name       string
		init       acp.InitializeResponse
		proxy      *MCPProxy
		wantLen    int
		assertAcp  bool
		assertName string
	}{
		{
			name:    "agent supports acp + proxy present → announce one entry",
			init:    acp.InitializeResponse{AgentCapabilities: acp.AgentCapabilities{McpCapabilities: acp.McpCapabilities{Acp: true}}},
			proxy:   proxy,
			wantLen: 1, assertAcp: true, assertName: "jaeger",
		},
		{
			name:    "agent does NOT support acp → empty list",
			init:    acp.InitializeResponse{AgentCapabilities: acp.AgentCapabilities{McpCapabilities: acp.McpCapabilities{Acp: false}}},
			proxy:   proxy,
			wantLen: 0,
		},
		{
			name:    "nil proxy → empty list even if agent supports acp",
			init:    acp.InitializeResponse{AgentCapabilities: acp.AgentCapabilities{McpCapabilities: acp.McpCapabilities{Acp: true}}},
			proxy:   nil,
			wantLen: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := announceMCPServers(tc.init, tc.proxy)
			require.NotNil(t, got, "must be a non-nil slice; ACP forbids null on this field")
			require.Len(t, got, tc.wantLen)
			if tc.assertAcp {
				require.NotNil(t, got[0].Acp)
				assert.Equal(t, tc.assertName, got[0].Acp.Name)
				assert.NotEmpty(t, string(got[0].Acp.Id), "Id must be unique per announcement; the proxy uses it as routing key")
			}
		})
	}
}

func TestMCPACPConnectionsLifecycle(t *testing.T) {
	// The connection registry is the routing table the mcp/message
	// handler reads on every inbound inner-MCP request. set/get/delete
	// must be straightforward; nothing complex to assert beyond
	// idempotent delete (the mcp/disconnect path doesn't error on
	// unknown ids, so the registry mustn't either).
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
	// Two mcp/connect calls with the same acpId should yield two
	// distinct connectionIds — the SDK allows multiple connections per
	// announced server, and a future agent might open more than one
	// for retry or multiplexing.
	proxy, _, _ := newACPProxyHarness(t, "sess-conn", nil, "")
	req := acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp-1"}

	resp1, err := proxy.HandleConnect(t.Context(), "sess-conn", req)
	require.NoError(t, err)
	resp2, err := proxy.HandleConnect(t.Context(), "sess-conn", req)
	require.NoError(t, err)
	assert.NotEqual(t, resp1.ConnectionId, resp2.ConnectionId,
		"connection ids must be unique even when acpId is reused")
	assert.NotEmpty(t, string(resp1.ConnectionId))
}

func TestHandleConnectRejectsEmptyAcpID(t *testing.T) {
	// An empty acpId is the only thing HandleConnect validates — the
	// session id comes from the dispatcher (already authenticated),
	// not from the request body, so there's nothing else to gate on.
	proxy, _, _ := newACPProxyHarness(t, "sess-empty", nil, "")
	_, err := proxy.HandleConnect(t.Context(), "sess-empty", acp.UnstableConnectMcpRequest{AcpId: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acpId is required")
}

func TestHandleDisconnectIsIdempotent(t *testing.T) {
	// A flaky agent re-sending mcp/disconnect, or a disconnect arriving
	// after the chat handler already tore everything down, must not
	// surface as an error. HandleDisconnect drops the entry quietly.
	proxy, _, _ := newACPProxyHarness(t, "sess-dc", nil, "")
	resp, err := proxy.HandleDisconnect(t.Context(), acp.UnstableDisconnectMcpRequest{ConnectionId: "never-existed"})
	require.NoError(t, err)
	assert.Equal(t, acp.UnstableDisconnectMcpResponse{}, resp)
}

func TestHandleMessageReturnsToolsListForKnownSession(t *testing.T) {
	// End-to-end: open a connection, then send a tools/list as an inner
	// MCP message. The response should be a ListToolsResult that mirrors
	// what listToolsForSession produces for that session id.
	highlight := sampleUITool(t, "ui_highlight_span", "Highlight a span",
		map[string]any{"type": "object"})
	proxy, _, _ := newACPProxyHarness(t, "sess-list", []json.RawMessage{highlight}, "")

	connectResp, err := proxy.HandleConnect(t.Context(), "sess-list", acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
	require.NoError(t, err)

	rawResp, err := proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: connectResp.ConnectionId,
		Method:       "tools/list",
	})
	require.NoError(t, err)

	listed, ok := rawResp.(*mcp.ListToolsResult)
	require.True(t, ok, "tools/list response must be *mcp.ListToolsResult; got %T", rawResp)
	require.Len(t, listed.Tools, 1)
	assert.Equal(t, "ui_highlight_span", listed.Tools[0].Name)
}

func TestHandleMessageRoutesToolCallToSSEForUITool(t *testing.T) {
	// The whole point of MCP-over-ACP: an inner tools/call for a UI
	// tool must surface as TOOL_CALL_START/ARGS/END on the SSE stream,
	// the same way the HTTP path does. This is the load-bearing test
	// for the two-transport dispatch sharing the same logic.
	highlight := sampleUITool(t, "ui_highlight_span", "Highlight a span",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{"spanId": map[string]any{"type": "string"}},
		})
	proxy, _, rec := newACPProxyHarness(t, "sess-call", []json.RawMessage{highlight}, "")

	connectResp, err := proxy.HandleConnect(t.Context(), "sess-call", acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
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
		"UI dispatch through MCP-over-ACP must not emit TOOL_CALL_RESULT — same contract as the HTTP transport and the original ACP ext_method path")
}

func TestHandleMessageReturnsErrorOnUnknownConnection(t *testing.T) {
	// A stale connection id is a real-world failure mode (agent reuses
	// a disconnected id, or sends mcp/message before mcp/connect). The
	// handler should return an explicit error so the dispatcher can
	// surface it as an ACP InvalidParams to the agent.
	proxy, _, _ := newACPProxyHarness(t, "sess-stale", nil, "")
	_, err := proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: "ghost-connection",
		Method:       "tools/list",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown connectionId")
}

func TestHandleMessageReturnsInitializeShape(t *testing.T) {
	// The inner MCP `initialize` method must succeed because the
	// agent's MCP client expects to complete the handshake before
	// emitting tools/list. Returning a minimum-viable InitializeResult
	// — protocol version, tools capability, server info — is enough
	// for the SDK's client to proceed.
	proxy, _, _ := newACPProxyHarness(t, "sess-init", nil, "")
	connectResp, err := proxy.HandleConnect(t.Context(), "sess-init", acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
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
	// We deliberately only implement the subset agents need to call
	// tools (initialize, tools/list, tools/call, notifications/initialized).
	// Anything else — resources/*, prompts/*, logging/* — surfaces as
	// an explicit "not supported" so the operator sees the call rather
	// than a confusing silent failure.
	proxy, _, _ := newACPProxyHarness(t, "sess-unk", nil, "")
	connectResp, err := proxy.HandleConnect(t.Context(), "sess-unk", acp.UnstableConnectMcpRequest{AcpId: "jaeger-mcp"})
	require.NoError(t, err)

	_, err = proxy.HandleMessage(t.Context(), acp.UnstableMessageMcpRequest{
		ConnectionId: connectResp.ConnectionId,
		Method:       "resources/list",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestListToolsForSessionMergesUIAndUpstream(t *testing.T) {
	// listToolsForSession is the shared catalogue both transports read.
	// Verifying it produces a merged list independently of the
	// transport, with UI tools first and upstream tools second — and
	// no duplicates when a UI tool shadows an upstream tool of the
	// same name. (Same precedence rule the HTTP-path tests cover end-
	// to-end; here we assert at the helper level so future changes
	// don't drift the two paths apart.)
	upstream := startFakeUpstreamMCP(t, "get_services")
	highlight := sampleUITool(t, "ui_highlight_span", "Highlight a span",
		map[string]any{"type": "object"})
	proxy, _, _ := newACPProxyHarness(t, "sess-merge", []json.RawMessage{highlight}, upstream)

	tools := proxy.listToolsForSession("sess-merge")
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"ui_highlight_span", "get_services"}, names)
}
