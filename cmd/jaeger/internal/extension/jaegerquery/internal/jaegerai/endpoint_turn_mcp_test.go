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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func rawUITool(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"name":        name,
		"description": name + " description",
		"parameters":  map[string]any{"type": "object"},
	})
	require.NoError(t, err)
	return b
}

// sharedMCPHandler builds the shared telemetry MCP endpoint the query server owns
// and hands to the gateway — the same server both mounts are served from.
func sharedMCPHandler(t *testing.T) *mcptools.Handler {
	t.Helper()
	params := mcptools.DefaultServerParams()
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := mcptools.NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), params)
	return h
}

// turnMCPServer mounts a real turn-scoped handler with one active turn
// ("sess-1") holding the given UI tools, and returns the test HTTP server plus
// the recorder backing that turn's SSE stream (to observe UI-tool dispatch).
func turnMCPServer(t *testing.T, uiTools []json.RawMessage) (ts *httptest.Server, rec *httptest.ResponseRecorder, routeID string) {
	t.Helper()
	turns := newTurnRegistry()
	rec = httptest.NewRecorder()
	routeID = registerTurn(turns, newAGUIRun(context.Background(), rec, "thread", "run"), uiTools)

	h := turnScopedEndpointBuilder{
		shared: sharedMCPHandler(t),
		turns:  turns,
		logger: telemetry.NoopSettings().Logger,
	}.build()
	mux := http.NewServeMux()
	h.registerRoutes(mux)

	ts = httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, rec, routeID
}

func connectTurnMCP(t *testing.T, ts *httptest.Server, path string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL + path,
		HTTPClient: ts.Client(),
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })
	return session
}

// listToolNames connects an MCP client to path and returns the tool names the
// server advertises there.
func listToolNames(t *testing.T, ts *httptest.Server, path string) []string {
	t.Helper()
	session := connectTurnMCP(t, ts, path)
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	return toolNames(listed.Tools)
}

func TestTurnScopedEndpointServesTelemetryPlusUITools(t *testing.T) {
	ts, _, routeID := turnMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})

	got := listToolNames(t, ts, "/api/ai/mcp/"+routeID+"/")
	assert.Contains(t, got, "get_services", "built-in telemetry tools must be advertised")
	assert.Contains(t, got, UIToolPrefix+"show_chart", "the turn's UI tools must be advertised under the namespace")
	assert.NotContains(t, got, "show_chart", "the bare frontend name is not an MCP tool")
}

// The turn-scoped endpoint issues no session id, not even to a client on an
// older protocol revision whose initialize would open a session elsewhere, so
// nothing outlives a request that another request could name. A client that
// sends one anyway is served as if it had not, and the GET and DELETE of older
// revisions are refused: there is no standing stream and no session to end.
func TestTurnScopedEndpointIssuesNoSessionID(t *testing.T) {
	ts, _, routeID := turnMCPServer(t, nil)

	post := func(body, sessionID string) *http.Response {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/ai/mcp/"+routeID+"/",
			strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	// This is the initialize an agent on a pre-2026-07-28 revision opens with,
	// and the one request a server that keeps sessions answers with a session id.
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"0.0.0"}}}`
	resp := post(initialize, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Mcp-Session-Id"), "the turn-scoped endpoint issues no session id")
	toolsList := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	assert.Equal(t, http.StatusOK, post(toolsList, "some-session").StatusCode, "a session id sent anyway is ignored")

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req, err := http.NewRequestWithContext(context.Background(), method, ts.URL+"/api/ai/mcp/"+routeID+"/", http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Mcp-Session-Id", "some-session")
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "%s on a live turn: no standing stream and no session to end", method)
	}
}

// TestSharedMCPHandlerServesTelemetryOnly pins the shared mount's contract while a
// chat gateway runs on the same server. Building the gateway installs
// uiToolsMiddleware on that server, so the middleware now sees the external clients'
// requests too; it must resolve no turn for them and leave the tool list alone. The
// turn below is live throughout, so a leak would show up as its UI tool appearing on
// the shared mount.
func TestSharedMCPHandlerServesTelemetryOnly(t *testing.T) {
	shared := sharedMCPHandler(t)
	h := NewHandler(HandlerParams{
		Logger:             telemetry.NoopSettings().Logger,
		AgentURL:           "ws://127.0.0.1:1",
		MaxRequestBodySize: 1 << 20,
		MCP:                shared,
	})
	routeID := registerTurn(h.mcp.turns, testAGUIRun(), []json.RawMessage{rawUITool(t, "show_chart")})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	// The shared mount, as jaeger-query's mountSharedMCP registers it.
	mux.Handle("/api/ai/mcp/", http.StripPrefix("/api/ai/mcp", shared))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	require.Contains(t, listToolNames(t, ts, "/api/ai/mcp/"+routeID+"/"), UIToolPrefix+"show_chart",
		"precondition: the turn's UI tool is live on the turn-scoped mount")

	got := listToolNames(t, ts, "/api/ai/mcp/")
	assert.Contains(t, got, "get_services", "the shared mount serves the telemetry tools")
	assert.NotContains(t, got, UIToolPrefix+"show_chart", "the shared mount must never advertise a turn's UI tools")
}

func TestTurnScopedEndpointDispatchesUIToolToStream(t *testing.T) {
	ts, rec, routeID := turnMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})
	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+routeID+"/")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      UIToolPrefix + "show_chart",
		Arguments: map[string]any{"series": "latency"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// The UI-tool call was dispatched to the browser over the turn's SSE
	// stream under the bare name the frontend registered.
	assert.Contains(t, rec.Body.String(), `"toolCallName":"show_chart"`)
	assert.NotContains(t, rec.Body.String(), UIToolPrefix)
}

// TestTurnScopedEndpointIsolatesTurns is the key guarantee of the single
// shared server: two turns declaring different UI tools each see only their
// own (plus the shared telemetry tools), and a UI-tool call reaches only the
// calling turn's stream. If the middleware ever resolved the wrong turn
// from the request context, this would cross the wires.
func TestTurnScopedEndpointIsolatesTurns(t *testing.T) {
	turns := newTurnRegistry()
	recA, recB := httptest.NewRecorder(), httptest.NewRecorder()
	idA := registerTurn(turns, newAGUIRun(context.Background(), recA, "ta", "ra"), []json.RawMessage{rawUITool(t, "chart_a")})
	idB := registerTurn(turns, newAGUIRun(context.Background(), recB, "tb", "rb"), []json.RawMessage{rawUITool(t, "chart_b")})

	h := turnScopedEndpointBuilder{
		shared: sharedMCPHandler(t),
		turns:  turns,
		logger: telemetry.NoopSettings().Logger,
	}.build()
	mux := http.NewServeMux()
	h.registerRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	sessionA := connectTurnMCP(t, ts, "/api/ai/mcp/"+idA+"/")
	sessionB := connectTurnMCP(t, ts, "/api/ai/mcp/"+idB+"/")

	listA, err := sessionA.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	listB, err := sessionB.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	namesA, namesB := toolNames(listA.Tools), toolNames(listB.Tools)

	assert.Contains(t, namesA, UIToolPrefix+"chart_a")
	assert.NotContains(t, namesA, UIToolPrefix+"chart_b", "turn A must not see turn B's UI tools")
	assert.Contains(t, namesB, UIToolPrefix+"chart_b")
	assert.NotContains(t, namesB, UIToolPrefix+"chart_a", "turn B must not see turn A's UI tools")
	assert.Contains(t, namesA, "get_services", "both turns still see the shared telemetry tools")
	assert.Contains(t, namesB, "get_services")

	_, err = sessionA.CallTool(context.Background(), &mcp.CallToolParams{Name: UIToolPrefix + "chart_a"})
	require.NoError(t, err)
	assert.Contains(t, recA.Body.String(), "chart_a", "the dispatch reaches the calling turn's stream")
	assert.NotContains(t, recB.Body.String(), "chart_a", "the other turn's stream is untouched")
}

func TestTurnScopedEndpointRejectsUnknownTurn(t *testing.T) {
	ts, _, _ := turnMCPServer(t, nil)
	for _, p := range []string{"/api/ai/mcp/ghost/mcp", "/api/ai/mcp/ghost"} {
		t.Run(p, func(t *testing.T) {
			resp, err := ts.Client().Get(ts.URL + p)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}
