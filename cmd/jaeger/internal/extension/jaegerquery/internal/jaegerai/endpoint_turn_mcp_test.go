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

// turnMCPServer mounts a real turn-scoped handler with one active turn
// ("sess-1") holding the given UI tools, and returns the test HTTP server plus
// the recorder backing that turn's SSE stream (to observe UI-tool dispatch).
func turnMCPServer(t *testing.T, uiTools []json.RawMessage) (ts *httptest.Server, rec *httptest.ResponseRecorder, routeID string) {
	t.Helper()
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	turns := newTurnRegistry()
	rec = httptest.NewRecorder()
	routeID = registerTurn(turns, newStreamingClient(context.Background(), rec, "thread", "run"), uiTools)

	h := newTurnScopedEndpoint(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), turns, "", zap.NewNop())
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

func TestTurnScopedEndpointServesTelemetryPlusUITools(t *testing.T) {
	ts, _, routeID := turnMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})
	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+routeID+"/")

	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}
	assert.Contains(t, got, "get_services", "built-in telemetry tools must be advertised")
	assert.Contains(t, got, "show_chart", "the turn's UI tools must be advertised")
}

func TestTurnScopedEndpointDispatchesUIToolToStream(t *testing.T) {
	ts, rec, routeID := turnMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})
	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+routeID+"/")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "show_chart",
		Arguments: map[string]any{"series": "latency"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// The UI-tool call was dispatched to the browser over the turn's SSE
	// stream — the recorder should carry the TOOL_CALL_* frames for it.
	assert.Contains(t, rec.Body.String(), "show_chart")
}

// TestTurnScopedEndpointIsolatesTurns is the key guarantee of the single
// shared server: two turns declaring different UI tools each see only their
// own (plus the shared telemetry tools), and a UI-tool call reaches only the
// calling turn's stream. If the middleware ever resolved the wrong turn
// from the request context, this would cross the wires.
func TestTurnScopedEndpointIsolatesTurns(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	turns := newTurnRegistry()
	recA, recB := httptest.NewRecorder(), httptest.NewRecorder()
	idA := registerTurn(turns, newStreamingClient(context.Background(), recA, "ta", "ra"), []json.RawMessage{rawUITool(t, "chart_a")})
	idB := registerTurn(turns, newStreamingClient(context.Background(), recB, "tb", "rb"), []json.RawMessage{rawUITool(t, "chart_b")})

	h := newTurnScopedEndpoint(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), turns, "", zap.NewNop())
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

	assert.Contains(t, namesA, "chart_a")
	assert.NotContains(t, namesA, "chart_b", "turn A must not see turn B's UI tools")
	assert.Contains(t, namesB, "chart_b")
	assert.NotContains(t, namesB, "chart_a", "turn B must not see turn A's UI tools")
	assert.Contains(t, namesA, "get_services", "both turns still see the shared telemetry tools")
	assert.Contains(t, namesB, "get_services")

	_, err = sessionA.CallTool(context.Background(), &mcp.CallToolParams{Name: "chart_a"})
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
