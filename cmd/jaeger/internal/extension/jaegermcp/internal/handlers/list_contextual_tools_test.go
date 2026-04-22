// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

type mockContextualToolsProvider struct {
	tools map[string][]any
}

func (m *mockContextualToolsProvider) GetContextualToolsForSession(sessionID string) []any {
	return m.tools[sessionID]
}

func TestListContextualToolsHandler_ReturnsTools(t *testing.T) {
	provider := &mockContextualToolsProvider{
		tools: map[string][]any{
			"sess-1": {map[string]any{"name": "tool-a"}},
		},
	}
	handler := NewListContextualToolsHandler(provider)

	_, output, err := handler(context.Background(), nil, types.ListContextualToolsInput{
		SessionID: "sess-1",
	})
	require.NoError(t, err)
	require.Len(t, output.Tools, 1)
	assert.Equal(t, "tool-a", output.Tools[0].(map[string]any)["name"])
}

func TestListContextualToolsHandler_ReturnsNilForUnknownSession(t *testing.T) {
	provider := &mockContextualToolsProvider{
		tools: map[string][]any{},
	}
	handler := NewListContextualToolsHandler(provider)

	_, output, err := handler(context.Background(), nil, types.ListContextualToolsInput{
		SessionID: "unknown",
	})
	require.NoError(t, err)
	assert.Nil(t, output.Tools)
}