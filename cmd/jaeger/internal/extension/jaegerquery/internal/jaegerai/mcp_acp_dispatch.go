// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// MCPOverACP version constants the gateway advertises when an agent
// sends `initialize` via the mcp/message envelope. We claim the
// current MCP protocol revision so SDK-based agent MCP clients accept
// the response; the gateway is otherwise faithful to MCP semantics
// (tools/list and tools/call shapes are produced by the same SDK that
// the agent's MCP client decodes them with, so wire compatibility is
// inherited rather than re-implemented).
const mcpOverACPProtocolVersion = "2025-06-18"

// mcpACPConnection holds the state the gateway tracks for one
// MCP-over-ACP connection. There is one entry per successful
// mcp/connect; mcp/disconnect removes it.
//
// The AG-UI session id is recorded so mcp/message can route the inner
// MCP call to the right UI-tool store and SSE stream — the gateway
// dispatches by session id the same way the HTTP transport does.
type mcpACPConnection struct {
	connectionID string
	acpID        string // the McpServerAcp.Id we announced for this session
	sessionID    string // AG-UI/ACP session id the connection belongs to
}

// mcpACPConnections is a tiny thread-safe registry mapping
// connectionId → mcpACPConnection. Reads from mcp/message dispatch
// (one HTTP/JSON-RPC request from the agent ⇒ one Lookup); writes from
// mcp/connect and mcp/disconnect. RWMutex over the small map is the
// right granularity — contention is microseconds and the map never
// grows beyond a few entries per active chat.
type mcpACPConnections struct {
	mu   sync.RWMutex
	byID map[string]*mcpACPConnection
}

func newMCPACPConnections() *mcpACPConnections {
	return &mcpACPConnections{byID: make(map[string]*mcpACPConnection)}
}

func (c *mcpACPConnections) set(conn *mcpACPConnection) {
	c.mu.Lock()
	c.byID[conn.connectionID] = conn
	c.mu.Unlock()
}

func (c *mcpACPConnections) get(connectionID string) *mcpACPConnection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byID[connectionID]
}

func (c *mcpACPConnections) delete(connectionID string) {
	c.mu.Lock()
	delete(c.byID, connectionID)
	c.mu.Unlock()
}

// mcpACPConnIDSeq is a process-wide counter used to mint unique
// connection ids. Combined with a nanosecond timestamp and the
// originating acpId, collisions are impossible across goroutines.
var mcpACPConnIDSeq atomic.Uint64

// gatewayHTTPHost is the scheme+authority the gateway announces for
// its own HTTP MCP endpoint. Hardcoded for the POC; a follow-up will
// derive it from the jaeger_query http endpoint config and expose an
// ai.gateway_public_url override. The full URL is assembled as
// "<gatewayHTTPHost><basePath><routeMCPPrefix><uuid>/" so the
// announced path tracks whatever basePath the operator mounts
// jaeger-query under.
const gatewayHTTPHost = "http://127.0.0.1:16686"

// announceMCPServers builds the NewSessionRequest.mcpServers list for a
// chat turn. The chat handler has already minted the per-turn UUID via
// MCPProxy.NewSessionUUID(); this function bakes it into the announced
// URLs (HTTP) and ids (ACP) so the sidecar can dial either transport
// and the gateway can resolve uuid → sessionID through one map.
//
// Two entries are emitted, capability-gated:
//
//   - HTTP entry, gated on mcp_capabilities.http (defaults true for
//     most agents). The URL embeds the UUID at the path segment the
//     gateway's MCPProxy.ServeHTTP looks up. This is the transport
//     the Gemini sidecar uses today.
//   - ACP entry, gated on mcp_capabilities.acp. The id is the same
//     UUID so HandleConnect can resolve through the same uuid→session
//     map. Used by MCP-over-ACP-aware agents (none in-tree today; the
//     mock-acp-agent demonstrates the path).
//
// proxy == nil disables both announcements — the dispatcher's mcp/*
// handlers degrade to MethodNotFound and the HTTP path 404s in tandem.
//
// The returned slice is always non-nil — ACP requires the field to be
// a JSON array, not null.
func announceMCPServers(init acp.InitializeResponse, proxy *MCPProxy, uuid string) []acp.McpServer {
	servers := make([]acp.McpServer, 0, 2)
	if proxy == nil || uuid == "" {
		return servers
	}
	caps := init.AgentCapabilities.McpCapabilities
	if caps.Http {
		servers = append(servers, acp.McpServer{
			Http: &acp.McpServerHttpInline{
				Name:    "jaeger",
				Url:     gatewayHTTPHost + proxy.basePath + routeMCPPrefix + uuid + "/",
				Headers: []acp.HttpHeader{},
			},
		})
	}
	if caps.Acp {
		servers = append(servers, acp.McpServer{
			Acp: &acp.McpServerAcpInline{
				Id:   acp.McpServerAcpId(uuid),
				Name: "jaeger",
			},
		})
	}
	return servers
}

func newMCPACPConnectionID(acpID string) string {
	return fmt.Sprintf("%s-conn-%d-%d", acpID, time.Now().UnixNano(), mcpACPConnIDSeq.Add(1))
}

// HandleConnect implements the gateway side of the mcp/connect ACP
// request. The agent sends this when it sees an `Acp`-variant McpServer
// in NewSessionRequest.mcpServers and wants to open a callback channel
// for it.
//
// The acpId in the request is the per-turn UUID the gateway announced
// in mcpServers. We resolve it through the same uuid→session map the
// HTTP transport uses, so both transports share one source of truth for
// "which AG-UI session does this connection belong to?" An unknown
// UUID — already-expired turn, malicious caller, or just-too-early
// dial before RegisterUUIDForSession ran — is rejected; the agent
// can retry later or fall back to the HTTP transport.
func (p *MCPProxy) HandleConnect(_ context.Context, req acp.UnstableConnectMcpRequest) (acp.UnstableConnectMcpResponse, error) {
	acpID := string(req.AcpId)
	if acpID == "" {
		return acp.UnstableConnectMcpResponse{}, errors.New("mcp/connect: acpId is required")
	}
	sessionID := p.resolveSessionFromUUID(acpID)
	if sessionID == "" {
		return acp.UnstableConnectMcpResponse{}, fmt.Errorf("mcp/connect: unknown acpId %q", acpID)
	}
	conn := &mcpACPConnection{
		connectionID: newMCPACPConnectionID(acpID),
		acpID:        acpID,
		sessionID:    sessionID,
	}
	p.acpConnections.set(conn)
	p.logger.Debug(
		"mcp-over-acp connection opened",
		zap.String("session_id", sessionID),
		zap.String("acp_id", acpID),
		zap.String("connection_id", conn.connectionID),
	)
	return acp.UnstableConnectMcpResponse{ConnectionId: acp.UnstableMcpConnectionId(conn.connectionID)}, nil
}

// HandleDisconnect drops the registered connection. Idempotent — an
// unknown connection id is a no-op rather than an error so a stray
// disconnect from a flaky agent doesn't poison the chat turn.
func (p *MCPProxy) HandleDisconnect(_ context.Context, req acp.UnstableDisconnectMcpRequest) (acp.UnstableDisconnectMcpResponse, error) {
	connectionID := string(req.ConnectionId)
	if connectionID != "" {
		p.acpConnections.delete(connectionID)
		p.logger.Debug(
			"mcp-over-acp connection closed",
			zap.String("connection_id", connectionID),
		)
	}
	return acp.UnstableDisconnectMcpResponse{}, nil
}

// HandleMessage dispatches an inner MCP request that the agent has
// tunnelled through the ACP channel. The response value is whatever
// JSON-marshalable shape the inner MCP method expects — the SDK
// re-encodes it on the way back to the agent so we can return typed
// SDK structs.
//
// The set of supported inner methods is the minimum needed for an
// agent to discover and call tools: initialize, tools/list, tools/call.
// Notifications (e.g. notifications/initialized) are accepted with
// a nil response so the agent's handshake completes; resources/*,
// prompts/*, logging/* etc. are intentionally not implemented (and
// will surface as method-not-found, the same way the HTTP path does
// implicitly).
func (p *MCPProxy) HandleMessage(ctx context.Context, req acp.UnstableMessageMcpRequest) (acp.UnstableMessageMcpResponse, error) {
	conn := p.acpConnections.get(string(req.ConnectionId))
	if conn == nil {
		return nil, fmt.Errorf("mcp/message: unknown connectionId %q", string(req.ConnectionId))
	}

	switch req.Method {
	case "initialize":
		// MCP's initialize: announce protocol version + tool capability.
		// The agent's MCP client expects this exact shape; we leverage
		// the SDK's typed struct so JSON marshalling matches whatever
		// the SDK's client decodes.
		return &mcp.InitializeResult{
			ProtocolVersion: mcpOverACPProtocolVersion,
			Capabilities:    &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
			ServerInfo: &mcp.Implementation{
				Name:    mcpServerName,
				Version: mcpServerVersion,
			},
		}, nil

	case "tools/list":
		return &mcp.ListToolsResult{Tools: p.listToolsForSession(conn.sessionID)}, nil

	case "tools/call":
		var params mcp.CallToolParams
		if rawParams, err := json.Marshal(req.Params); err == nil {
			_ = json.Unmarshal(rawParams, &params)
		}
		if params.Name == "" {
			return errorResult("tools/call: name is required"), nil
		}
		// CallToolParams.Arguments comes in as `any`; convert to raw
		// JSON so dispatchUITool / forwardToUpstream see the same
		// shape they get from the HTTP path's req.Params.Arguments
		// (json.RawMessage). Keeps the internal dispatch layer
		// transport-agnostic.
		var rawArgs json.RawMessage
		if params.Arguments != nil {
			encoded, err := json.Marshal(params.Arguments)
			if err != nil {
				return errorResult(fmt.Sprintf("tools/call: cannot marshal arguments: %v", err)), nil
			}
			rawArgs = encoded
		}
		return p.callToolForSession(ctx, conn.sessionID, params.Name, rawArgs)

	case "notifications/initialized":
		// Fire-and-forget handshake notification. Nil response is fine
		// — the SDK encoder elides null result fields.
		return nil, nil

	default:
		return nil, fmt.Errorf("mcp/message: method %q not supported on this MCP-over-ACP endpoint", req.Method)
	}
}

// listToolsForSession returns the combined UI + upstream tool list the
// gateway exposes to the agent for the given AG-UI session. Both the
// HTTP path (via the SDK's per-session *mcp.Server's tools/list) and
// the ACP path (via HandleMessage's "tools/list" case) call this so
// they advertise the same catalogue.
func (p *MCPProxy) listToolsForSession(sessionID string) []*mcp.Tool {
	uiTools := p.ctxTools.GetContextualToolsForSession(sessionID)
	out := make([]*mcp.Tool, 0, len(uiTools)+len(p.upstreamToolList()))

	for _, raw := range uiTools {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		if name == "" {
			continue
		}
		description, _ := entry["description"].(string)
		out = append(out, &mcp.Tool{
			// Names are prefixed on the wire to keep frontend-supplied
			// names from colliding with built-in telemetry tool names.
			// callToolForSession strips the prefix on lookup so the
			// internal ctxTools store stays keyed by the raw name.
			Name:        applyUIToolPrefix(name),
			Description: description,
			InputSchema: normalizeUIToolSchema(entry["parameters"]),
		})
	}

	for _, tool := range p.upstreamToolList() {
		if tool == nil || tool.Name == "" {
			continue
		}
		// Same precedence rule as serverForRequest: UI tools win on
		// name collision. Both sides are compared by raw frontend name
		// (ctxTools stores unprefixed entries; upstream tools never
		// carry the ui_ prefix) — the prefix only appears on the wire
		// output of tools/list above.
		if uiToolNameRegistered(uiTools, tool.Name) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

// callToolForSession is the shared "what does this tool name mean for
// this session?" router. Called by HandleMessage; the HTTP path uses
// the SDK's per-tool handlers (uiToolHandler / upstreamToolHandler)
// which already encode the same routing decision through their
// per-tool closures, so the two paths emit identical behaviour.
//
// toolName arrives on the wire as the agent saw it in tools/list —
// UI tools therefore carry the UIToolPrefix and must be stripped
// before consulting ctxTools (which stores the raw frontend name).
func (p *MCPProxy) callToolForSession(ctx context.Context, sessionID, toolName string, rawArgs json.RawMessage) (*mcp.CallToolResult, error) {
	// UI tools first — frontend-declared tools take precedence over
	// upstream entries with the same name.
	if stripped, ok := stripUIToolPrefixOK(toolName); ok {
		if uiToolNameRegistered(p.ctxTools.GetContextualToolsForSession(sessionID), stripped) {
			return p.dispatchUITool(ctx, sessionID, stripped, rawArgs), nil
		}
	}
	for _, tool := range p.upstreamToolList() {
		if tool != nil && tool.Name == toolName {
			return p.forwardToUpstream(ctx, sessionID, toolName, rawArgs)
		}
	}
	return errorResult(fmt.Sprintf("unknown tool %q", toolName)), nil
}

// applyUIToolPrefix is the idempotent inverse of stripUIToolPrefixOK.
// Names already starting with UIToolPrefix pass through unchanged so a
// frontend that pre-prefixes (or a misconfigured caller) doesn't end
// up with double-prefixed wire shapes like "ui_ui_highlight_span".
func applyUIToolPrefix(name string) string {
	if strings.HasPrefix(name, UIToolPrefix) {
		return name
	}
	return UIToolPrefix + name
}

// stripUIToolPrefixOK returns (stripped, true) when name starts with
// UIToolPrefix and the remainder is non-empty; otherwise (name, false).
// The boolean lets callToolForSession distinguish "this is a UI tool
// candidate" from "this is a telemetry tool" without re-checking
// HasPrefix afterwards.
func stripUIToolPrefixOK(name string) (string, bool) {
	if stripped, ok := strings.CutPrefix(name, UIToolPrefix); ok && stripped != "" {
		return stripped, true
	}
	return name, false
}
