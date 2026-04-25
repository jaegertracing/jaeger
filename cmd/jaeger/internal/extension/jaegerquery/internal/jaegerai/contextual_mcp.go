// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// contextualMCPIDPathValue is the name of the path wildcard carrying the
// per-turn contextual MCP id in the gateway MCP URL template
// (/api/ai/mcp/{contextual_mcp_id}). This id is minted by the chat
// handler for each ACP turn and used as the key into ContextualToolsStore;
// it is *not* the ACP session id (which is assigned by the sidecar after
// NewSession returns and is therefore not available when the URL must be
// embedded in NewSessionRequest.McpServers).
const contextualMCPIDPathValue = "contextual_mcp_id"

// NewContextualMCPHandler returns an http.Handler that serves a per-turn
// MCP endpoint exposing only the AG-UI tools the frontend attached to the
// given turn. The returned handler must be mounted at a path that captures
// the contextual MCP id as a {contextual_mcp_id} wildcard (e.g.
// "/api/ai/mcp/{contextual_mcp_id}").
//
// The sidecar connects to this URL after receiving it via
// NewSessionRequest.McpServers so a single agent instance can dispatch
// against different frontend tool lists concurrently without races.
//
// tools/call is stubbed pending the browser relay: the handler acknowledges
// the call as an error with a human-readable explanation.
func NewContextualMCPHandler(logger *zap.Logger, store *ContextualToolsStore) http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			contextualMCPID := r.PathValue(contextualMCPIDPathValue)
			return newContextualMCPServer(logger, store, contextualMCPID)
		},
		&mcp.StreamableHTTPOptions{
			JSONResponse: false,
			Stateless:    true,
		},
	)
}

// newContextualMCPServer materialises a one-shot MCP server exposing the
// tools recorded under contextualMCPID. Unknown ids, an empty id, or a nil
// store all yield a server with no tools registered — callers that
// constructed the handler without a store get an empty tool list rather
// than a nil-pointer panic on the first request.
func newContextualMCPServer(logger *zap.Logger, store *ContextualToolsStore, contextualMCPID string) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "jaeger-ai-contextual",
			Version: "0.1.0",
		},
		nil,
	)

	if store == nil || contextualMCPID == "" {
		return server
	}

	tools := store.GetContextualToolsForID(contextualMCPID)
	for _, entry := range tools {
		tool, ok := toMCPTool(entry)
		if !ok {
			logger.Warn("skipping contextual tool with unexpected shape",
				zap.String("contextual_mcp_id", contextualMCPID))
			continue
		}
		server.AddTool(tool, newContextualToolHandler(tool.Name))
	}

	return server
}

// toMCPTool translates one AG-UI tool entry into an mcp.Tool. AG-UI tools
// use {name, description, parameters} where parameters is a JSON Schema
// object; the sidecar only needs those three fields to dispatch.
// Any other shape is rejected so the sidecar does not see a half-formed
// tool that Gemini cannot sanely invoke.
//
// When the AG-UI entry omits `parameters`, the schema defaults to an empty
// object schema — mcp.Server.AddTool panics on a nil InputSchema.
func toMCPTool(entry any) (*mcp.Tool, bool) {
	obj, ok := entry.(map[string]any)
	if !ok {
		return nil, false
	}
	name, ok := obj["name"].(string)
	if !ok || name == "" {
		return nil, false
	}
	// mcp.Server.AddTool panics on a nil InputSchema, so AG-UI entries
	// without `parameters` get a minimal empty-object schema.
	tool := &mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}
	if desc, ok := obj["description"].(string); ok {
		tool.Description = desc
	}
	if params, ok := obj["parameters"]; ok && params != nil {
		tool.InputSchema = params
	}
	return tool, true
}

// newContextualToolHandler returns an MCP tool handler that reports the
// browser relay is not yet available. A future change will replace this
// with a call that rides the ACP `session/request_permission` round-trip
// back to the frontend.
func newContextualToolHandler(toolName string) mcp.ToolHandler {
	return func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "contextual tool '" + toolName + "' requires browser relay, which is not yet wired"},
			},
		}, nil
	}
}
