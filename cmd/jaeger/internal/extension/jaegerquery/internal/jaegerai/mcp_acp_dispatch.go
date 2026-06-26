// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// mcpACPServerIDSeq is the sibling counter for the McpServerAcp.Id we
// announce in NewSessionRequest.mcpServers. Distinct from the
// connection counter because the announced id outlives any single
// connection (the agent could in principle open multiple connections
// to the same announced server).
var mcpACPServerIDSeq atomic.Uint64

// announceMCPServers builds the NewSessionRequest.mcpServers list for a
// chat turn. Today there's at most one entry — the MCP-over-ACP
// callback the agent uses to reach the gateway-side MCP server — and
// we add it only when:
//
//   - the chat handler has a proxy wired in (proxy != nil), AND
//   - the agent advertised mcp_capabilities.acp = true in its
//     InitializeResponse.
//
// When either condition is false we return an empty slice so the
// gateway behaves exactly as it did before this feature: no MCP
// servers announced over the ACP wire. Agents that can't speak
// MCP-over-ACP keep using the HTTP MCP endpoint instead, and the
// dispatcher's mcp/* method handlers degrade to MethodNotFound for
// them too (proxy stays unused on that path).
//
// The returned slice is always non-nil — ACP requires the field to be
// a JSON array, not null.
func announceMCPServers(init acp.InitializeResponse, proxy *MCPProxy) []acp.McpServer {
	servers := make([]acp.McpServer, 0, 1)
	if proxy == nil || !init.AgentCapabilities.McpCapabilities.Acp {
		return servers
	}
	acpID := fmt.Sprintf("jaeger-mcp-%d-%d", time.Now().UnixNano(), mcpACPServerIDSeq.Add(1))
	servers = append(servers, acp.McpServer{
		Acp: &acp.McpServerAcpInline{
			Id:   acp.McpServerAcpId(acpID),
			Name: "jaeger",
		},
	})
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
// The gateway announces exactly one ACP McpServer per session and
// generates a fresh acpId for it before session/new is dispatched, so
// there's nothing to validate the inbound acpId against beyond
// non-emptiness — the per-connection sessionID comes from the chat
// handler's atomic (sessionIDRef in newDispatcher), not from the
// request. Future-proof: the SDK allows multiple connections per
// announced server, so we don't reject a second mcp/connect for the
// same acpId.
func (p *MCPProxy) HandleConnect(_ context.Context, sessionID string, req acp.UnstableConnectMcpRequest) (acp.UnstableConnectMcpResponse, error) {
	acpID := string(req.AcpId)
	if acpID == "" {
		return acp.UnstableConnectMcpResponse{}, errors.New("mcp/connect: acpId is required")
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
	out := make([]*mcp.Tool, 0, len(uiTools)+len(p.upstreamTools))

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
			Name:        name,
			Description: description,
			InputSchema: normalizeUIToolSchema(entry["parameters"]),
		})
	}

	for _, tool := range p.upstreamTools {
		if tool == nil || tool.Name == "" {
			continue
		}
		// Same precedence rule as serverForRequest: UI tools win on
		// name collision.
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
func (p *MCPProxy) callToolForSession(ctx context.Context, sessionID, toolName string, rawArgs json.RawMessage) (*mcp.CallToolResult, error) {
	// UI tools first — frontend-declared tools take precedence over
	// upstream entries with the same name (see serverForRequest's
	// collision rule).
	if uiToolNameRegistered(p.ctxTools.GetContextualToolsForSession(sessionID), toolName) {
		return p.dispatchUITool(sessionID, toolName, rawArgs), nil
	}
	for _, tool := range p.upstreamTools {
		if tool != nil && tool.Name == toolName {
			return p.forwardToUpstream(ctx, toolName, rawArgs)
		}
	}
	return errorResult(fmt.Sprintf("unknown tool %q", toolName)), nil
}
