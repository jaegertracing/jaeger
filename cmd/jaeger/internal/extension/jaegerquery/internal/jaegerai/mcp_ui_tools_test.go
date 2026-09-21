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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
	assert.Equal(t, "ui_show_chart", descs[0].Name)
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
	assert.Equal(t, []string{"ui_show_chart", "ui_highlight"}, toolNames(descs), "repeated names collapse to one entry")
}

func TestAppendUITools(t *testing.T) {
	// No UI tools → telemetry list returned unchanged.
	telemetry := []*mcp.Tool{{Name: "get_services"}, {Name: "search_traces"}}
	assert.Equal(t, telemetry, appendUITools(telemetry, &turnState{}, zap.NewNop()))

	// A UI tool shadows a same-named telemetry tool (single entry, UI wins); the
	// unrelated telemetry tool is kept and the new UI tool is appended.
	sess := &turnState{uiTools: []json.RawMessage{
		rawUITool(t, "search_traces"), // shadows the telemetry tool of the same name
		rawUITool(t, "show_chart"),
	}}
	merged := appendUITools([]*mcp.Tool{{Name: "get_services"}, {Name: "search_traces"}}, sess, zap.NewNop())
	assert.Equal(t, []string{"get_services", "ui_search_traces", "ui_show_chart"}, toolNames(merged),
		"shadowed telemetry tool is dropped and UI tools appended, one entry per name")
}

func TestSessionDeclaredUITool(t *testing.T) {
	sess := &turnState{uiTools: []json.RawMessage{
		rawUITool(t, "show_chart"),
		json.RawMessage(`not json`), // malformed entry never matches
	}}
	assert.True(t, turnDeclaredUITool(sess, "show_chart"))
	assert.False(t, turnDeclaredUITool(sess, "get_services"))
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
	res = emitUIToolCall(stream, "show_chart", json.RawMessage(`{"series":"latency"}`))
	require.False(t, res.IsError)
	assert.Contains(t, rec.Body.String(), "show_chart", "the tool-call lifecycle is emitted to the browser stream")
}

func TestNewUIToolCallID(t *testing.T) {
	a := newUIToolCallID("show_chart")
	b := newUIToolCallID("show_chart")
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
		assert.Equal(t, []string{"get_services", "ui_show_chart"}, toolNames(res.(*mcp.ListToolsResult).Tools))
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
			Params: &mcp.CallToolParamsRaw{Name: "show_chart", Arguments: json.RawMessage(`{"a":1}`)},
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

	t.Run("tools/call telemetry tool returns error", func(t *testing.T) {
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, errors.New("upstream failed")
		}
		_, err := mw(next)(ctx, methodCallTool, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "get_services"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream failed")
	})

	t.Run("tools/call telemetry tool returns unexpected type", func(t *testing.T) {
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.ListToolsResult{}, nil
		}
		_, err := mw(next)(ctx, methodCallTool, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "get_services"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected response type")
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

func TestUIToolsMiddleware_WithTracer(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer("test")
	mw := uiToolsMiddleware(newTurnRegistry(), zap.NewNop(), tracer)
	assert.NotNil(t, mw)
}

func TestDispatchUITool_ErrorAndNilTracer(t *testing.T) {
	rec := httptest.NewRecorder()
	stream := newStreamingClient(context.Background(), rec, "t", "r")
	turn := &turnState{stream: stream}

	// Nil tracer with valid call
	res := dispatchUITool(context.Background(), turn, "ui_render", json.RawMessage(`{"a":1}`), nil, zap.NewNop())
	assert.False(t, res.IsError)

	// Error result with invalid JSON arguments
	resErr := dispatchUITool(context.Background(), turn, "ui_render", json.RawMessage(`{invalid`), nil, zap.NewNop())
	assert.True(t, resErr.IsError)
}

func TestForwardToUpstream_NilTracer(t *testing.T) {
	res, err := forwardToUpstream(context.Background(), nil, "search_traces", nil, func(_ context.Context) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}, nil, zap.NewNop())
	require.NoError(t, err)
	assert.False(t, res.IsError)
}

func TestDispatchUITool_Tracing(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	rec := httptest.NewRecorder()
	stream := newStreamingClient(context.Background(), rec, "t", "r")
	turn := &turnState{stream: stream}

	res := dispatchUITool(context.Background(), turn, "ui_show_chart", json.RawMessage(`{"k":"v"}`), tracer, zap.NewNop())
	require.False(t, res.IsError)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "dispatchUITool ui_show_chart", spans[0].Name)

	hasToolName := false
	hasGenAIOp := false
	for _, attr := range spans[0].Attributes {
		if attr.Key == "gen_ai.tool.name" && attr.Value.AsString() == "ui_show_chart" {
			hasToolName = true
		}
		if attr.Key == "gen_ai.operation.name" && attr.Value.AsString() == "execute_tool" {
			hasGenAIOp = true
		}
	}
	assert.True(t, hasToolName, "must record gen_ai.tool.name")
	assert.True(t, hasGenAIOp, "must record gen_ai.operation.name execute_tool")
}

func TestForwardToUpstream_TracingAndSSE(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	rec := httptest.NewRecorder()
	stream := newStreamingClient(context.Background(), rec, "t", "r")
	turn := &turnState{stream: stream, disableStandaloneSSE: false}

	called := false
	res, err := forwardToUpstream(context.Background(), turn, "search_traces", json.RawMessage(`{"limit":10}`), func(_ context.Context) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "traces-found"}},
		}, nil
	}, tracer, zap.NewNop())

	require.NoError(t, err)
	assert.True(t, called)
	require.False(t, res.IsError)
	assert.Contains(t, rec.Body.String(), "search_traces", "SSE emission must occur for upstream tool")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "forwardToUpstream search_traces", spans[0].Name)
}

func TestForwardToUpstream_DisableStandaloneSSE(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	rec := httptest.NewRecorder()
	stream := newStreamingClient(context.Background(), rec, "t", "r")
	turn := &turnState{stream: stream, disableStandaloneSSE: true}

	_, err := forwardToUpstream(context.Background(), turn, "search_traces", nil, func(_ context.Context) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}, tracer, zap.NewNop())

	require.NoError(t, err)
	assert.Empty(t, rec.Body.String(), "SSE emission must be suppressed when DisableStandaloneSSE is true")
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
