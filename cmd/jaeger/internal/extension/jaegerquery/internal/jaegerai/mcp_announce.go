// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	acp "github.com/coder/acp-go-sdk"
)

// mcpServerName is the human-readable name the gateway gives its MCP server in
// the session/new announcement. Agents surface it in logs and tool namespacing,
// so keep it stable.
const mcpServerName = "jaeger"

// announceMCPServers builds the NewSessionRequest.mcpServers list that tells the
// sidecar how to reach this turn's turn-scoped MCP endpoint. Without it the
// endpoint is dormant: it serves the telemetry tools and the turn's UI tools, but
// no agent knows the URL to dial.
//
// The announcement is capability-gated on the agent's InitializeResponse. ACP
// requires an agent to opt into each McpServer variant, and announcing one it
// cannot consume would make it fail the session. The gateway offers a single
// variant today:
//
//   - mcpCapabilities.http → MCP over streamable HTTP at
//     "<baseURL><basePath>/api/ai/mcp/<mcpRouteID>/".
//
// baseURL is the resolved ai.mcp_base_url: an explicit override, or the gateway's
// inferred localhost address when the sidecar is co-located (see
// AIConfig.resolveMCPBaseURL). It is empty only when the sidecar is remote and no
// override is set — the gateway then cannot infer an address the sidecar can reach
// it on, and announcing an unreachable one is worse than announcing none (the
// agent dials it and fails mid-turn). With an empty baseURL no HTTP server is
// announced and the chat turn still works (just without tools).
func announceMCPServers(caps acp.AgentCapabilities, baseURL, basePath, mcpRouteID string) []acp.McpServer {
	if mcpRouteID == "" || baseURL == "" || !caps.McpCapabilities.Http {
		return []acp.McpServer{}
	}
	return []acp.McpServer{{
		Http: &acp.McpServerHttpInline{
			Type:    "http",
			Name:    mcpServerName,
			Url:     baseURL + basePath + routeMCPPrefix + mcpRouteID + "/",
			Headers: []acp.HttpHeader{},
		},
	}}
}
