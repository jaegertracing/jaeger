// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
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

func TestParseUITool(t *testing.T) {
	_, ok := parseUITool(json.RawMessage(`not json`))
	assert.False(t, ok, "malformed JSON is rejected")

	_, ok = parseUITool(mustJSON(t, map[string]any{"description": "no name"}))
	assert.False(t, ok, "a tool without a name is rejected")

	def, ok := parseUITool(mustJSON(t, map[string]any{
		"name": "show_chart", "description": "d",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
	}))
	require.True(t, ok)
	assert.Equal(t, "show_chart", def.name)
	assert.Equal(t, "d", def.description)
	assert.Equal(t, "object", def.schema["type"])

	// A non-object schema is normalized to the default object schema.
	def, ok = parseUITool(mustJSON(t, map[string]any{"name": "x", "parameters": "bad"}))
	require.True(t, ok)
	assert.Equal(t, map[string]any{"type": "object"}, def.schema)
}

func TestUIToolDescriptorsSkipsMalformed(t *testing.T) {
	sess := &turnState{uiTools: []json.RawMessage{
		json.RawMessage(`not json`),                     // unmarshal error → skipped
		mustJSON(t, map[string]any{"description": "x"}), // empty name → skipped
		mustJSON(t, map[string]any{"name": "show_chart", "description": "d", "parameters": map[string]any{"type": "object"}}),
	}}
	descs := uiToolDescriptors(sess, zap.NewNop())
	require.Len(t, descs, 1, "only the well-formed tool is described")
	assert.Equal(t, UIToolPrefix+"show_chart", descs[0].Name,
		"advertised namespaced so a frontend name cannot collide with a telemetry tool")
	assert.Equal(t, "d", descs[0].Description)
	assert.Equal(t, map[string]any{"type": "object"}, descs[0].InputSchema)
}

func TestUIToolDescriptorsDeduplicatesNames(t *testing.T) {
	sess := &turnState{uiTools: []json.RawMessage{
		rawUITool(t, "show_chart"),
		rawUITool(t, "show_chart"), // a frontend that declares the same tool twice
		rawUITool(t, "highlight"),
	}}
	descs := uiToolDescriptors(sess, zap.NewNop())
	assert.Equal(t, []string{UIToolPrefix + "show_chart", UIToolPrefix + "highlight"}, toolNames(descs),
		"repeated names collapse to one entry")
}

func TestAppendUITools(t *testing.T) {
	// No UI tools → telemetry list returned unchanged.
	telemetry := []*mcp.Tool{{Name: "get_services"}, {Name: "search_traces"}}
	assert.Equal(t, telemetry, appendUITools(telemetry, &turnState{}, zap.NewNop()))

	// A UI tool named after a telemetry tool no longer shadows it: the two live in
	// different namespaces, so both are advertised and the agent's telemetry query
	// still reaches Jaeger instead of being answered by the browser.
	sess := &turnState{uiTools: []json.RawMessage{
		rawUITool(t, "search_traces"), // same bare name as a telemetry tool
		rawUITool(t, "show_chart"),
	}}
	merged := appendUITools([]*mcp.Tool{{Name: "get_services"}, {Name: "search_traces"}}, sess, zap.NewNop())
	assert.Equal(t, []string{
		"get_services", "search_traces",
		UIToolPrefix + "search_traces", UIToolPrefix + "show_chart",
	}, toolNames(merged), "telemetry tools are kept intact and UI tools appended namespaced")
}

func TestSessionDeclaredUITool(t *testing.T) {
	sess := &turnState{uiTools: []json.RawMessage{
		rawUITool(t, "show_chart"),
		json.RawMessage(`not json`), // malformed entry never matches
	}}
	assert.True(t, turnDeclaredUITool(sess, UIToolPrefix+"show_chart"))
	// The bare name is a telemetry tool by construction, even though a UI tool
	// shares it — that separation is the point of the prefix.
	assert.False(t, turnDeclaredUITool(sess, "show_chart"))
	assert.False(t, turnDeclaredUITool(sess, UIToolPrefix+"get_services"))
	assert.False(t, turnDeclaredUITool(sess, UIToolPrefix))
	assert.False(t, turnDeclaredUITool(sess, ""))
}

func TestDispatchUIToolCall(t *testing.T) {
	// Nil stream (session ended mid-request) → error result, no panic.
	res := emitUIToolCall(nil, "show_chart", nil)
	assert.True(t, res.IsError, "a closed stream is reported as a tool error")

	// Invalid JSON arguments → error result.
	res = emitUIToolCall(testStreamingClient(), "show_chart", json.RawMessage(`{not json`))
	assert.True(t, res.IsError, "invalid JSON arguments must return an error result")

	// Success → non-error ack and the TOOL_CALL_* frames land on the stream.
	rec := httptest.NewRecorder()
	stream := newStreamingClient(context.Background(), rec, "t", "r")
	res = emitUIToolCall(stream, UIToolPrefix+"show_chart", json.RawMessage(`{"series":"latency"}`))
	require.False(t, res.IsError)
	body := rec.Body.String()
	assert.Contains(t, body, "show_chart", "the tool-call lifecycle is emitted to the browser stream")
	assert.NotContains(t, body, UIToolPrefix+"show_chart",
		"the browser knows the tool by the name it registered, not the namespaced one")
}

func TestNewUIToolCallID(t *testing.T) {
	a := newToolCallID("show_chart")
	b := newToolCallID("show_chart")
	assert.NotEqual(t, a, b, "ids are unique per call")
	assert.Contains(t, a, "show_chart", "the tool name is embedded for readable logs")
}

func TestUIDispatchMiddleware(t *testing.T) {
	rec := httptest.NewRecorder()
	turns := newTurnRegistry()
	routeID := registerTurn(turns, newStreamingClient(context.Background(), rec, "t", "r"),
		[]json.RawMessage{rawUITool(t, "show_chart")})
	mw := uiToolsMiddleware(turns, zap.NewNop())
	ctx := context.WithValue(context.Background(), mcpRouteIDContextKey{}, routeID)

	telemetryList := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "get_services"}}}, nil
	}

	t.Run("tools/list appends the session's UI tools", func(t *testing.T) {
		res, err := mw(telemetryList)(ctx, methodListTools, &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}})
		require.NoError(t, err)
		assert.Equal(t, []string{"get_services", UIToolPrefix + "show_chart"},
			toolNames(res.(*mcp.ListToolsResult).Tools))
	})

	t.Run("tools/list with no active session is telemetry-only", func(t *testing.T) {
		res, err := mw(telemetryList)(context.Background(), methodListTools, &mcp.ListToolsRequest{})
		require.NoError(t, err)
		assert.Equal(t, []string{"get_services"}, toolNames(res.(*mcp.ListToolsResult).Tools))
	})

	t.Run("tools/list propagates a next error without appending", func(t *testing.T) {
		wantErr := errors.New("boom")
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return nil, wantErr }
		_, err := mw(next)(ctx, methodListTools, &mcp.ListToolsRequest{})
		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("tools/list passes a non-list result through", func(t *testing.T) {
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return &mcp.CallToolResult{}, nil }
		res, err := mw(next)(ctx, methodListTools, &mcp.ListToolsRequest{})
		require.NoError(t, err)
		assert.IsType(t, &mcp.CallToolResult{}, res)
	})

	t.Run("tools/call dispatches a UI tool without hitting telemetry", func(t *testing.T) {
		called := false
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		}
		res, err := mw(next)(ctx, methodCallTool, &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: UIToolPrefix + "show_chart", Arguments: json.RawMessage(`{"a":1}`)},
		})
		require.NoError(t, err)
		assert.False(t, called, "a UI tool must not fall through to the telemetry handlers")
		assert.False(t, res.(*mcp.CallToolResult).IsError)
		assert.Contains(t, rec.Body.String(), "show_chart")
	})

	t.Run("tools/call passes a telemetry tool through", func(t *testing.T) {
		called := false
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		}
		_, err := mw(next)(ctx, methodCallTool, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "get_services"}})
		require.NoError(t, err)
		assert.True(t, called, "non-UI tool calls fall through to telemetry")
	})

	t.Run("tools/call with nil params returns a tool error, not a passthrough", func(t *testing.T) {
		called := false
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		}
		res, err := mw(next)(ctx, methodCallTool, &mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.False(t, called, "a params-less call must not reach the downstream handler, which would nil-deref")
		assert.True(t, res.(*mcp.CallToolResult).IsError)
	})

	t.Run("tools/call passes a mismatched request type through", func(t *testing.T) {
		called := false
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		}
		_, err := mw(next)(ctx, methodCallTool, &mcp.ListToolsRequest{})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("unrelated method passes through", func(t *testing.T) {
		called := false
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) { called = true; return nil, nil }
		_, err := mw(next)(ctx, "initialize", &mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, called)
	})
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// TestToolResultTextRendersError pins that a failed telemetry tool shows why in
// the chat: with no result text the tool card would end empty and look like a
// hang rather than a failure.
func TestToolResultTextRendersError(t *testing.T) {
	got := toolResultText(nil, errors.New("storage unavailable"))
	assert.Equal(t, "tool call failed: storage unavailable", got)
}

// TestToolResultTextIgnoresNonToolResult covers the shape the SDK should never
// hand us for tools/call. Returning "" omits TOOL_CALL_RESULT rather than
// rendering Go's default formatting of an unexpected type into the chat.
func TestToolResultTextIgnoresNonToolResult(t *testing.T) {
	assert.Empty(t, toolResultText(&mcp.ListToolsResult{}, nil))
}

// TestToolResultTextJoinsTextContent pins the multi-block case: a tool returning
// several text blocks renders as one result rather than only its first block.
func TestToolResultTextJoinsTextContent(t *testing.T) {
	res := &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: "first"},
		&mcp.ImageContent{},
		&mcp.TextContent{Text: "second"},
	}}
	assert.Equal(t, "first\nsecond", toolResultText(res, nil))
}

// TestEmitTelemetryToolCallWithoutStreamStillRunsTool covers a turn that ended
// mid-request. The call is already in flight for the agent, so the tool must still
// run and return — only the browser-side reporting is lost.
func TestEmitTelemetryToolCallWithoutStreamStillRunsTool(t *testing.T) {
	want := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}
	called := false
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		called = true
		return want, nil
	}
	call := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "get_services"}}

	got, err := emitTelemetryToolCall(context.Background(), methodCallTool, call, call, nil, next)

	require.NoError(t, err)
	assert.True(t, called, "the tool must run even with no stream to report to")
	assert.Same(t, want, got)
}
