// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNormalizeUIToolSchema(t *testing.T) {
	obj := map[string]any{"type": "object", "properties": map[string]any{}}
	assert.Equal(t, obj, normalizeUIToolSchema(obj), "an object schema passes through unchanged")

	def := map[string]any{"type": "object"}
	assert.Equal(t, def, normalizeUIToolSchema(nil), "nil degrades to the empty object schema")
	assert.Equal(t, def, normalizeUIToolSchema("not-a-map"), "non-map degrades to the empty object schema")
	assert.Equal(t, def, normalizeUIToolSchema(map[string]any{"type": "string"}), "non-object type degrades to the empty object schema")
}

func TestSafeAddTool(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)

	require.NoError(t, safeAddTool(srv, &mcp.Tool{
		Name:        "ok",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, nil }))

	// A nil input schema makes Server.AddTool panic; safeAddTool converts it to
	// an error instead of crashing.
	err := safeAddTool(srv, &mcp.Tool{Name: "bad"}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, nil })
	require.Error(t, err)
}

func TestUIToolHandlerRejectsInvalidArgs(t *testing.T) {
	stream := testStreamingClient()
	h := uiToolHandler(stream, "show_chart")

	res, err := h(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "show_chart", Arguments: json.RawMessage(`{not json`)},
	})
	require.NoError(t, err)
	require.True(t, res.IsError, "invalid JSON arguments must return an error result")
}

func TestAddUIToolsSkipsMalformedAndDuplicates(t *testing.T) {
	rec := httptest.NewRecorder()
	sess := &session{
		stream: newStreamingClient(context.Background(), rec, "t", "r"),
		uiTools: []json.RawMessage{
			json.RawMessage(`not valid json`),            // unmarshal error → skipped
			json.RawMessage(`{"description":"no name"}`), // empty name → skipped
			mustJSON(t, map[string]any{"name": "show_chart", "parameters": map[string]any{"type": "object"}}),
			mustJSON(t, map[string]any{"name": "show_chart", "parameters": map[string]any{"type": "object"}}), // duplicate name — must not panic
		},
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	// Must not panic despite the malformed/duplicate entries.
	require.NotPanics(t, func() { addUITools(srv, sess, zap.NewNop()) })
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
