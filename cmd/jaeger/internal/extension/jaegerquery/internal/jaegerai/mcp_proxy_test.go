// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mcpProxyFixture wires the MCP proxy against in-memory stores and an
// httptest.Server, plus an MCP client session pointed at the test
// endpoint. Holding the streaming client + its httptest.Recorder lets
// tests assert what AG-UI events the proxy pushed onto the SSE stream
// when an MCP tool call arrived.
type mcpProxyFixture struct {
	sessionID string
	ctxTools  *ContextualToolsStore
	streams   *SessionStreams
	proxy     *MCPProxy

	stream    *streamingClient
	sseRec    *httptest.ResponseRecorder
	server    *httptest.Server
	mcpClient *mcp.ClientSession
}

// newMCPProxyFixture sets up the proxy mounted on a real httptest.Server,
// registers a live streamingClient under sessionID, populates
// ContextualToolsStore with `uiTools`, and connects an MCP client. All
// resources are torn down via t.Cleanup.
//
// upstreamURL controls the upstream MCP server the proxy dials at
// construction. Pass "" to skip the dial — appropriate for tests that
// only exercise the UI-tool path. Pass an httptest.Server URL (with the
// SDK's MCP HTTP route appended) to exercise the upstream-forwarding
// path.
//
// uiTools is the raw JSON shape the frontend sends — an array of
// `{"name", "description", "parameters"}` objects.
func newMCPProxyFixture(t *testing.T, sessionID string, uiTools []json.RawMessage, upstreamURL string) *mcpProxyFixture {
	t.Helper()

	f := &mcpProxyFixture{
		sessionID: sessionID,
		ctxTools:  NewContextualToolsStore(),
		streams:   NewSessionStreams(),
	}
	if len(uiTools) > 0 {
		f.ctxTools.SetForSession(sessionID, uiTools)
	}

	// streamingClient under test holds an httptest.Recorder so the test
	// can inspect TOOL_CALL_* events afterwards. We open it with
	// startRun() so the messageID/run/thread state is initialised the
	// same way a real chat would do.
	f.sseRec = httptest.NewRecorder()
	f.stream = newStreamingClient(t.Context(), f.sseRec, "thread-test", "run-test")
	f.stream.startRun()
	f.streams.Set(sessionID, f.stream)

	f.proxy = newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", f.ctxTools, f.streams, upstreamURL)
	t.Cleanup(func() { _ = f.proxy.Close() })

	mux := http.NewServeMux()
	mux.Handle(routeMCPPrefix, f.proxy)

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	// Build the MCP client and connect to the session-scoped URL the
	// gateway would normally hand to the agent. The trailing slash
	// matches the path-shape ServeHTTP rewrites onto.
	client := mcp.NewClient(
		&mcp.Implementation{Name: "mcp-proxy-test", Version: "0.0.1"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   f.server.URL + routeMCPPrefix + sessionID + "/",
		HTTPClient: http.DefaultClient,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "MCP client should initialize against the proxy")
	t.Cleanup(func() { _ = session.Close() })
	f.mcpClient = session

	return f
}

func sampleUITool(t *testing.T, name, description string, params map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"name":        name,
		"description": description,
		"parameters":  params,
	})
	require.NoError(t, err)
	return raw
}

func TestMCPProxyServeHTTPRejectsMissingSessionID(t *testing.T) {
	// The proxy mounts at /api/ai/mcp/ as a prefix; an immediately-closed
	// path (no <sessionId> segment) is a malformed call and should be a
	// 400 so the operator notices in logs, not a silent 200.
	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", NewContextualToolsStore(), NewSessionStreams(), "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, routeMCPPrefix, http.NoBody)

	proxy.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"empty session id segment must be a 400 with a clear hint")
}

func TestMCPProxyServeHTTPRejectsBadPrefix(t *testing.T) {
	// A request that somehow reaches the proxy without the route prefix
	// (e.g. a misconfigured mux) should 404 instead of falling through
	// to the streamable handler with an empty session id, which would
	// register MCP sessions against the empty string.
	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", NewContextualToolsStore(), NewSessionStreams(), "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/not/the/prefix", http.NoBody)

	proxy.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestForwardToUpstreamWithoutSessionReturnsErrorResult(t *testing.T) {
	// When the initial upstream dial failed, p.upstream stays nil and
	// any forwardToUpstream call must produce an IsError CallToolResult
	// rather than nil-deref. Lets the gateway keep serving UI tools
	// while telemetry tools degrade gracefully.
	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", NewContextualToolsStore(), NewSessionStreams(), "")
	result, err := proxy.forwardToUpstream(t.Context(), "search_traces", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestForwardToUpstreamWithInvalidArgumentsReturnsErrorResult(t *testing.T) {
	// Even with a connected upstream session, malformed JSON arguments
	// (caller bug or wire corruption) must surface as IsError rather
	// than as a transport error or a nil-deref inside upstream.CallTool.
	// We can't easily synthesise a live upstream here, so we exercise
	// the parse branch with no upstream — same code path, same
	// IsError outcome.
	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), "", NewContextualToolsStore(), NewSessionStreams(), "")
	result, err := proxy.forwardToUpstream(t.Context(), "search_traces", json.RawMessage(`{not-json`))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestNewMCPProxyAcceptsNilLogger(t *testing.T) {
	// The constructor must tolerate a nil logger so callers in test
	// fixtures (and historical call sites that haven't been updated
	// for zap) don't have to construct a zap.NewNop themselves.
	proxy := newMCPProxyWithUpstream(t.Context(), nil, "", NewContextualToolsStore(), NewSessionStreams(), "")
	t.Cleanup(func() { _ = proxy.Close() })
	require.NotNil(t, proxy)
	require.NotNil(t, proxy.logger, "nil logger must be replaced with a no-op logger so log calls don't panic")
}

func TestMCPProxyServeHTTPStripsBasePath(t *testing.T) {
	// When jaeger-query runs behind an operator-configured base path
	// (e.g. "/jaeger"), the mux routes "<basePath>/api/ai/mcp/..." to
	// the proxy, so ServeHTTP must strip <basePath>+routeMCPPrefix —
	// not just routeMCPPrefix. Regression test for the earlier bug
	// where requests at a non-empty base path 404'd unconditionally.
	const basePath = "/jaeger"
	proxy := newMCPProxyWithUpstream(t.Context(), zap.NewNop(), basePath, NewContextualToolsStore(), NewSessionStreams(), "")

	t.Run("session id segment present at base path → reaches handler", func(t *testing.T) {
		// With a session id and basePath stripped correctly, control
		// reaches the wrapped streamable handler; a bare GET there
		// returns 405 (the handler accepts POST/GET-as-SSE, not arbitrary
		// methods on /). A 404 here would mean the prefix didn't match.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, basePath+routeMCPPrefix+"sess-1/", http.NoBody)
		proxy.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code,
			"basePath+routeMCPPrefix must be recognised; 404 indicates the bug where ServeHTTP only stripped routeMCPPrefix")
	})

	t.Run("missing base path → 404", func(t *testing.T) {
		// Without the operator's base path the request shouldn't reach
		// the proxy at all (the mux wouldn't route it). If it somehow
		// arrives here, 404 — don't accidentally treat the unprefixed
		// shape as valid.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, routeMCPPrefix+"sess-1/", http.NoBody)
		proxy.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code,
			"a request missing the configured base path must 404, not silently dispatch")
	})

	t.Run("empty session id at base path → 400", func(t *testing.T) {
		// Same as TestMCPProxyServeHTTPRejectsMissingSessionID but
		// proves the 400 fires when basePath is present too.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, basePath+routeMCPPrefix, http.NoBody)
		proxy.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestMCPProxyToolsListAdvertisesFrontendUITools(t *testing.T) {
	// The proxy's tools/list must mirror what the frontend declared in
	// the chat request — that's what makes the gateway's MCP endpoint a
	// drop-in replacement for "however the agent used to find tools."
	highlight := sampleUITool(
		t, "ui_highlight_span", "Highlight a span in the timeline",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spanId": map[string]any{"type": "string"},
			},
		},
	)
	openTrace := sampleUITool(
		t, "ui_open_trace", "Open a trace in a new tab",
		map[string]any{"type": "object"},
	)

	f := newMCPProxyFixture(t, "sess-1", []json.RawMessage{highlight, openTrace}, "")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := f.mcpClient.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err, "tools/list against the proxy must succeed")

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"ui_highlight_span", "ui_open_trace"}, names,
		"tools/list must include every UI tool the session registered")
}

func TestMCPProxyToolsListReturnsEmptyWhenNoSessionTools(t *testing.T) {
	// A connection that comes in before SetForSession ran (race, or a
	// stale URL the operator pasted) must not error — just advertise an
	// empty toolset. The agent will then have nothing Jaeger-specific
	// to call, which is the right degraded behaviour.
	f := newMCPProxyFixture(t, "sess-empty", nil, "")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := f.mcpClient.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	assert.Empty(t, result.Tools)
}

func TestMCPProxyToolsCallDispatchesToSSEStream(t *testing.T) {
	// This is the load-bearing scenario: an agent invoking a UI tool
	// over MCP must show up as TOOL_CALL_START / ARGS / END on the SSE
	// stream the chat handler is keeping open, and the agent must get
	// back a non-error CallToolResult so the LLM loop continues. No
	// TOOL_CALL_RESULT is emitted to the browser — the browser is the
	// executor, not a passive consumer.
	highlight := sampleUITool(
		t, "ui_highlight_span", "Highlight a span",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spanId": map[string]any{"type": "string"},
			},
			"required": []string{"spanId"},
		},
	)
	f := newMCPProxyFixture(t, "sess-call", []json.RawMessage{highlight}, "")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	args := map[string]any{"spanId": "abc123"}
	result, err := f.mcpClient.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ui_highlight_span",
		Arguments: args,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError, "synthetic ack must not be an error")
	require.NotEmpty(t, result.Content, "ack must carry at least one content block")

	// Inspect the SSE stream the chat handler would have written to.
	// startRun emitted RUN_STARTED already; EmitContextualToolCall
	// appends START / ARGS / END for this dispatch.
	body := f.sseRec.Body.String()
	events := parseSSEEvents(t, body)
	types := eventTypes(events)
	require.Contains(t, types, "TOOL_CALL_START")
	require.Contains(t, types, "TOOL_CALL_ARGS")
	require.Contains(t, types, "TOOL_CALL_END")
	assert.NotContains(t, types, "TOOL_CALL_RESULT",
		"server must not emit TOOL_CALL_RESULT for a UI-dispatched tool — "+
			"the browser is the executor and a server-side result would short-circuit assistant-ui")

	// Find the START event and check the toolCallName flowed through.
	for _, evt := range events {
		if evt["type"] == "TOOL_CALL_START" {
			assert.Equal(t, "ui_highlight_span", evt["toolCallName"])
			break
		}
	}

	// Find the ARGS event and verify the agent's payload reached the
	// SSE stream verbatim — that's what the browser uses to dispatch
	// to its locally-registered execute().
	for _, evt := range events {
		if evt["type"] == "TOOL_CALL_ARGS" {
			delta, ok := evt["delta"].(string)
			require.True(t, ok)
			assert.JSONEq(t, `{"spanId":"abc123"}`, delta,
				"TOOL_CALL_ARGS.delta must carry the JSON-encoded MCP arguments")
			break
		}
	}
}

func TestMCPProxyToolsCallWithoutStreamReturnsError(t *testing.T) {
	// If the chat session ended (or never began) but the agent still
	// holds a stale MCP URL, tools/call must return IsError rather than
	// panic or quietly drop the dispatch — the LLM needs a real signal
	// to stop retrying.
	highlight := sampleUITool(
		t, "ui_highlight_span", "Highlight a span",
		map[string]any{"type": "object"},
	)
	f := newMCPProxyFixture(t, "sess-no-stream", []json.RawMessage{highlight}, "")
	// Simulate the chat ending: deregister the streaming client.
	f.streams.Delete(f.sessionID)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := f.mcpClient.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ui_highlight_span",
		Arguments: map[string]any{},
	})
	require.NoError(t, err, "MCP transport must not error; we use IsError to signal stale session")
	assert.True(t, result.IsError, "expected IsError so the agent knows to stop")
	require.NotEmpty(t, result.Content)
}

func TestNormalizeUIToolSchemaDefendsAgainstMalformedFrontend(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want map[string]any
	}{
		{
			"nil falls back to empty object schema",
			nil,
			map[string]any{"type": "object"},
		},
		{
			"non-object value falls back to empty object schema",
			"not-a-schema",
			map[string]any{"type": "object"},
		},
		{
			"object without type:object falls back so MCP AddTool doesn't panic",
			map[string]any{"type": "string"},
			map[string]any{"type": "object"},
		},
		{
			"well-formed object schema is preserved",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"foo": map[string]any{"type": "string"},
				},
			},
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"foo": map[string]any{"type": "string"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeUIToolSchema(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSafeAddToolRecoversFromPanics(t *testing.T) {
	// AddTool panics on invalid schema. safeAddTool must turn that into
	// an error so one bad frontend tool entry can't take down the entire
	// per-session server during serverForRequest. Stripping InputSchema
	// triggers AddTool's own validation panic path.
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	err := safeAddTool(srv, &mcp.Tool{
		Name:        "broken_tool",
		InputSchema: nil, // panics inside AddTool
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	require.Error(t, err, "panic should surface as a returned error")
	assert.True(t, strings.Contains(err.Error(), "input schema") || strings.Contains(err.Error(), "AddTool"),
		"error should describe the schema problem so it's actionable in logs; got %q", err.Error())
}

func TestNewMCPToolCallIDIsStableAndUnique(t *testing.T) {
	// Tool call ids appear in TOOL_CALL_* events and are correlated by
	// assistant-ui's run aggregator; the only contract is unique-per-call
	// and human-readable enough to grep in logs. We assert the name
	// prefix is preserved and that two consecutive ids never collide.
	a := newMCPToolCallID("ui_highlight_span")
	b := newMCPToolCallID("ui_highlight_span")
	assert.True(t, strings.HasPrefix(a, "ui_highlight_span-"), "ids should start with the tool name")
	assert.NotEqual(t, a, b, "ids must be unique even for the same tool name within the same nanosecond")
}

// startFakeUpstreamMCP spins up an in-memory MCP server in front of an
// httptest.Server with a small synthetic tool whose handler echoes its
// arguments. Returns the MCP-protocol URL the proxy should dial. Used
// to test the upstream-forwarding path without depending on jaegermcp.
func startFakeUpstreamMCP(t *testing.T, toolName string) string {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "fake-upstream-mcp",
		Version: "0.0.1",
	}, nil)
	server.AddTool(&mcp.Tool{
		Name:        toolName,
		Description: "echoes the args it received as JSON in a text content block",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"echo": map[string]any{"type": "string"},
			},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Round-trip the args so the test can verify what reached the
		// upstream verbatim — that's the contract the forwarder owes.
		argsJSON := string(req.Params.Arguments)
		if argsJSON == "" {
			argsJSON = "null"
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "upstream got: " + argsJSON}},
		}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: false, Stateless: false, SessionTimeout: 30 * time.Second},
	)
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	return httpServer.URL
}

func TestMCPProxyToolsListIncludesUpstreamTelemetryTools(t *testing.T) {
	// The upstream-forwarding path should mirror the upstream's tools/list
	// alongside the per-session UI tools. The agent then sees one
	// combined catalogue and doesn't have to know about Jaeger-specific
	// dispatch internals.
	upstream := startFakeUpstreamMCP(t, "get_services")
	highlight := sampleUITool(t, "ui_highlight_span", "Highlight a span",
		map[string]any{"type": "object"})

	f := newMCPProxyFixture(t, "sess-mixed", []json.RawMessage{highlight}, upstream)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := f.mcpClient.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"ui_highlight_span", "get_services"}, names,
		"tools/list must merge UI tools and upstream telemetry tools")
}

func TestMCPProxyToolsCallForwardsToUpstream(t *testing.T) {
	// Calls to telemetry tools must reach the upstream server unchanged
	// and the response must round-trip back to the agent. This is the
	// load-bearing scenario for "gateway as MCP egress" — every
	// telemetry call now goes through code we own (the gateway) instead
	// of the agent dialing jaegermcp directly.
	upstream := startFakeUpstreamMCP(t, "get_services")
	f := newMCPProxyFixture(t, "sess-fwd", nil, upstream)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := f.mcpClient.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_services",
		Arguments: map[string]any{"echo": "hello"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "upstream got:",
		"upstream's reply should reach the agent verbatim")
	assert.Contains(t, text.Text, `"echo":"hello"`,
		"args sent by the agent must reach the upstream unmodified")
}

func TestMCPProxyDegradesGracefullyWhenUpstreamUnreachable(t *testing.T) {
	// If the upstream MCP server is down at gateway startup, the proxy
	// must continue to serve UI tools and refuse upstream-shaped tool
	// calls with IsError rather than crashing or returning success.
	// Pointing at a closed httptest port simulates a stopped upstream
	// without making the test depend on TCP exhaustion timing.
	closedServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := closedServer.URL
	closedServer.Close()

	highlight := sampleUITool(t, "ui_highlight_span", "Highlight a span",
		map[string]any{"type": "object"})
	f := newMCPProxyFixture(t, "sess-degraded", []json.RawMessage{highlight}, deadURL)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// tools/list must still succeed — UI tools remain available.
	result, err := f.mcpClient.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	assert.Equal(t, []string{"ui_highlight_span"}, names,
		"with upstream unreachable, the proxy advertises UI tools only")
}

func TestMCPProxyUITakesPrecedenceOverUpstreamOnNameCollision(t *testing.T) {
	// The collision rule: if the frontend declared a UI tool with the
	// same name as an upstream telemetry tool, the UI tool wins. The
	// frontend's per-turn declaration is more explicit than the static
	// upstream registration and we don't want a stale upstream entry
	// to suppress a UI surface the user just opted into.
	upstream := startFakeUpstreamMCP(t, "shared_name")
	uiSameName := sampleUITool(t, "shared_name", "frontend-declared UI tool",
		map[string]any{"type": "object"})

	f := newMCPProxyFixture(t, "sess-collide", []json.RawMessage{uiSameName}, upstream)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Only one tool with that name should be advertised — the UI one.
	listed, err := f.mcpClient.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	require.Len(t, listed.Tools, 1, "collision must not produce a duplicate")
	assert.Equal(t, "frontend-declared UI tool", listed.Tools[0].Description,
		"the UI tool's description must win over the upstream's")

	// Calling the shared name should dispatch via SSE (the UI path), not
	// hit the upstream. Verifying that the SSE recorder gained a
	// TOOL_CALL_START is enough — the upstream's echo would have come
	// back as the tool result text otherwise.
	_, err = f.mcpClient.CallTool(ctx, &mcp.CallToolParams{
		Name:      "shared_name",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	types := eventTypes(parseSSEEvents(t, f.sseRec.Body.String()))
	assert.Contains(t, types, "TOOL_CALL_START",
		"a colliding name must take the UI path (SSE dispatch), not the upstream path")
}
