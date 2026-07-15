// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

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

// sessionMCPServer mounts a real turn-scoped handler with one active session
// ("sess-1") holding the given UI tools, and returns the test HTTP server plus
// the recorder backing that session's SSE stream (to observe UI-tool dispatch).
func sessionMCPServer(t *testing.T, uiTools []json.RawMessage) (*httptest.Server, *httptest.ResponseRecorder) {
	t.Helper()
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	turns := newTurnRegistry()
	rec := httptest.NewRecorder()
	turns.set("sess-1", newStreamingClient(context.Background(), rec, "thread", "run"), uiTools)

	h := newTurnScopedEndpoint(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), turns, "", zap.NewNop())
	mux := http.NewServeMux()
	mux.Handle(routeMCPSession, h)
	mux.Handle(routeMCPSessionNoSlash, h)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, rec
}

func connectSessionMCP(t *testing.T, ts *httptest.Server, path string) *mcp.ClientSession {
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

func TestTurnScopedEndpointServesTelemetryPlusUITools(t *testing.T) {
	ts, _ := sessionMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})
	session := connectSessionMCP(t, ts, "/api/ai/mcp/sess-1/")

	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}
	assert.Contains(t, got, "get_services", "built-in telemetry tools must be advertised")
	assert.Contains(t, got, "show_chart", "the session's UI tools must be advertised")
}

func TestTurnScopedEndpointDispatchesUIToolToStream(t *testing.T) {
	ts, rec := sessionMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})
	session := connectSessionMCP(t, ts, "/api/ai/mcp/sess-1/")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "show_chart",
		Arguments: map[string]any{"series": "latency"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// The UI-tool call was dispatched to the browser over the session's SSE
	// stream — the recorder should carry the TOOL_CALL_* frames for it.
	assert.Contains(t, rec.Body.String(), "show_chart")
}

// TestTurnScopedEndpointIsolatesSessions is the key guarantee of the single
// shared server: two sessions declaring different UI tools each see only their
// own (plus the shared telemetry tools), and a UI-tool call reaches only the
// calling session's stream. If the middleware ever resolved the wrong session
// from the request context, this would cross the wires.
func TestTurnScopedEndpointIsolatesSessions(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	turns := newTurnRegistry()
	recA, recB := httptest.NewRecorder(), httptest.NewRecorder()
	turns.set("sess-a", newStreamingClient(context.Background(), recA, "ta", "ra"), []json.RawMessage{rawUITool(t, "chart_a")})
	turns.set("sess-b", newStreamingClient(context.Background(), recB, "tb", "rb"), []json.RawMessage{rawUITool(t, "chart_b")})

	h := newTurnScopedEndpoint(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), turns, "", zap.NewNop())
	mux := http.NewServeMux()
	mux.Handle(routeMCPSession, h)
	mux.Handle(routeMCPSessionNoSlash, h)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	sessionA := connectSessionMCP(t, ts, "/api/ai/mcp/sess-a/")
	sessionB := connectSessionMCP(t, ts, "/api/ai/mcp/sess-b/")

	listA, err := sessionA.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	listB, err := sessionB.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	namesA, namesB := toolNames(listA.Tools), toolNames(listB.Tools)

	assert.Contains(t, namesA, "chart_a")
	assert.NotContains(t, namesA, "chart_b", "session A must not see session B's UI tools")
	assert.Contains(t, namesB, "chart_b")
	assert.NotContains(t, namesB, "chart_a", "session B must not see session A's UI tools")
	assert.Contains(t, namesA, "get_services", "both sessions still see the shared telemetry tools")
	assert.Contains(t, namesB, "get_services")

	_, err = sessionA.CallTool(context.Background(), &mcp.CallToolParams{Name: "chart_a"})
	require.NoError(t, err)
	assert.Contains(t, recA.Body.String(), "chart_a", "the dispatch reaches the calling session's stream")
	assert.NotContains(t, recB.Body.String(), "chart_a", "the other session's stream is untouched")
}

func TestTurnScopedEndpointRejectsUnknownSession(t *testing.T) {
	ts, _ := sessionMCPServer(t, nil)
	for _, p := range []string{"/api/ai/mcp/ghost/mcp", "/api/ai/mcp/ghost"} {
		t.Run(p, func(t *testing.T) {
			resp, err := ts.Client().Get(ts.URL + p)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}
