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
)

const testAIMCPPath = "/api/ai/mcp/{contextual_mcp_id}"

// startContextualMCPTestServer mounts the handler on a test HTTP server and
// returns the session-scoped base URL plus the session id the caller
// seeded. Caller-provided tools are stored before the server comes up so
// initialisation sees a consistent snapshot.
func startContextualMCPTestServer(t *testing.T, store *ContextualToolsStore) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(testAIMCPPath, NewContextualMCPHandler(zap.NewNop(), store))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func connectContextualMCP(t *testing.T, srv *httptest.Server, sessionID string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/api/ai/mcp/" + sessionID,
		HTTPClient: srv.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestContextualMCPHandler_ListToolsReturnsSessionSnapshot(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart","description":"Draws a chart","parameters":{"type":"object","properties":{"kind":{"type":"string"}}}}`),
	})
	srv := startContextualMCPTestServer(t, store)
	session := connectContextualMCP(t, srv, "sess-abc")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, result.Tools, 1, "expected the one tool seeded for sess-abc")
	assert.Equal(t, "render_chart", result.Tools[0].Name)
	assert.Equal(t, "Draws a chart", result.Tools[0].Description)
}

func TestContextualMCPHandler_UnknownSessionYieldsNoTools(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})
	srv := startContextualMCPTestServer(t, store)
	session := connectContextualMCP(t, srv, "sess-nope")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Tools, "unknown session must not leak another session's snapshot")
}

func TestContextualMCPHandler_CallToolReturnsBrowserRelayStub(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})
	srv := startContextualMCPTestServer(t, store)
	session := connectContextualMCP(t, srv, "sess-abc")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "render_chart"})
	require.NoError(t, err, "stub tool calls must not surface as protocol errors")
	assert.True(t, result.IsError, "stub must mark the result as an error until browser relay is wired")

	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "render_chart")
	assert.Contains(t, text.Text, "browser relay")
}

func TestContextualMCPHandler_SkipsEntriesWithoutName(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("sess-abc", []json.RawMessage{
		json.RawMessage(`"not-an-object"`),
		json.RawMessage(`{"description":"missing name, must be skipped"}`),
		json.RawMessage(`{"name":"valid_tool","description":"kept"}`),
	})
	srv := startContextualMCPTestServer(t, store)
	session := connectContextualMCP(t, srv, "sess-abc")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, result.Tools, 1, "malformed entries must not be advertised")
	assert.Equal(t, "valid_tool", result.Tools[0].Name)
}

func TestContextualMCPHandler_EmptySessionIDYieldsNoTools(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("sess-abc", []json.RawMessage{
		json.RawMessage(`{"name":"render_chart"}`),
	})
	// Route that does NOT populate the contextual_mcp_id wildcard.
	mux := http.NewServeMux()
	mux.Handle("/api/ai/mcp", NewContextualMCPHandler(zap.NewNop(), store))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/api/ai/mcp",
		HTTPClient: srv.Client(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Tools, "missing session id must not leak tools from any session")
}

func TestContextualMCPHandler_NilStoreYieldsNoTools(t *testing.T) {
	// A nil store must not panic on first request — handler is exported and
	// callers (or tests) can construct it without a store. The expected
	// behavior is an empty tool list.
	srv := startContextualMCPTestServer(t, nil)
	session := connectContextualMCP(t, srv, "sess-anything")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Tools, "nil store must surface as empty tools, not a panic")
}
