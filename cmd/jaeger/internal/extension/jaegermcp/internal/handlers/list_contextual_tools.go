// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// contextualToolsProvider defines the interface for retrieving contextual tools.
type contextualToolsProvider interface {
	GetContextualToolsForSession(sessionID string) []any
}

// listContextualToolsHandler implements the list_contextual_tools MCP tool.
type listContextualToolsHandler struct {
	provider contextualToolsProvider
}

// NewListContextualToolsHandler creates a new list_contextual_tools handler
// and returns the handler function.
func NewListContextualToolsHandler(
	provider contextualToolsProvider,
) mcp.ToolHandlerFor[types.ListContextualToolsInput, types.ListContextualToolsOutput] {
	h := &listContextualToolsHandler{
		provider: provider,
	}
	return h.handle
}

// handle processes the list_contextual_tools tool request.
func (h *listContextualToolsHandler) handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input types.ListContextualToolsInput,
) (*mcp.CallToolResult, types.ListContextualToolsOutput, error) {
	return nil, types.ListContextualToolsOutput{
		Tools: h.provider.GetContextualToolsForSession(input.SessionID),
	}, nil
}
