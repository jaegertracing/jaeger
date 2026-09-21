// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestMCPServer() (*httptest.Server, *mcp.Server) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-upstream", Version: "1.0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "get_services",
		Description: "test tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "services-list"}},
		}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			JSONResponse: false,
			Stateless:    false,
		},
	)
	ts := httptest.NewServer(handler)
	return ts, server
}

func TestUpstreamClient_ConnectSuccess(t *testing.T) {
	ts, _ := newTestMCPServer()
	defer ts.Close()

	client := NewUpstreamClient(ts.URL, WithUpstreamLogger(zap.NewNop()))
	defer client.Close()

	err := client.Connect(context.Background())
	require.NoError(t, err)

	// ListTools works
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
	assert.Equal(t, "get_services", tools.Tools[0].Name)

	// CallTool works
	res, err := client.CallTool(context.Background(), "get_services", map[string]any{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, res.Content, 1)
	assert.Equal(t, "services-list", res.Content[0].(*mcp.TextContent).Text)
}

func TestUpstreamClient_DisableStandaloneSSEOption(t *testing.T) {
	client := NewUpstreamClient("http://127.0.0.1:16686", WithDisableStandaloneSSE(false))
	assert.False(t, client.disableStandaloneSSE)

	clientDefault := NewUpstreamClient("http://127.0.0.1:16686")
	assert.True(t, clientDefault.disableStandaloneSSE)
}

func TestUpstreamClient_SyncFirstDial_BackgroundRetry(t *testing.T) {
	var handlerRef atomic.Pointer[http.Handler]
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := handlerRef.Load()
		if h == nil {
			http.Error(w, "server unavailable", http.StatusServiceUnavailable)
			return
		}
		(*h).ServeHTTP(w, r)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	client := NewUpstreamClient(ts.URL,
		WithUpstreamLogger(zap.NewNop()),
		WithUpstreamRetryInterval(10*time.Millisecond),
		WithUpstreamMaxRetryInterval(15*time.Millisecond),
	)
	defer client.Close()

	// Initial sync dial should fail because server is unavailable
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := client.Connect(ctx)
	require.Error(t, err, "initial dial must return error on failure")

	// Allow retry loop to fail at least once so interval doubles and caps at maxInterval
	time.Sleep(30 * time.Millisecond)

	// Now make server available
	server := mcp.NewServer(&mcp.Implementation{Name: "test-upstream", Version: "1.0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "test_tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil
	})
	var actualHandler http.Handler = mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: false, Stateless: false},
	)
	handlerRef.Store(&actualHandler)

	// Wait for background retry loop to connect on its own (without calling ListTools)
	require.Eventually(t, func() bool {
		client.mu.RLock()
		defer client.mu.RUnlock()
		return client.session != nil
	}, 3*time.Second, 20*time.Millisecond)
}

func TestUpstreamClient_ErrSessionMissing_Reconnect(t *testing.T) {
	ts, server := newTestMCPServer()
	defer ts.Close()

	client := NewUpstreamClient(ts.URL, WithUpstreamLogger(zap.NewNop()))
	defer client.Close()

	err := client.Connect(context.Background())
	require.NoError(t, err)

	res, err := client.CallTool(context.Background(), "get_services", nil)
	require.NoError(t, err)
	assert.False(t, res.IsError)

	// Force close server-side sessions to trigger session missing error
	for sess := range server.Sessions() {
		_ = sess.Close()
	}

	// CallTool should detect missing session, re-dial, and succeed
	res2, err := client.CallTool(context.Background(), "get_services", nil)
	require.NoError(t, err)
	assert.False(t, res2.IsError)
	assert.Equal(t, "services-list", res2.Content[0].(*mcp.TextContent).Text)

	// Force close sessions again and verify ListTools also redials and succeeds
	for sess := range server.Sessions() {
		_ = sess.Close()
	}
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
}

func TestUpstreamClient_Close(t *testing.T) {
	ts, _ := newTestMCPServer()
	defer ts.Close()

	client := NewUpstreamClient(ts.URL)
	require.NoError(t, client.Connect(context.Background()))
	require.NoError(t, client.Close())

	// Connecting or calling after close fails
	assert.Error(t, client.Connect(context.Background()))
}

func TestUpstreamClient_WithUpstreamHTTPClient(t *testing.T) {
	ts, _ := newTestMCPServer()
	defer ts.Close()

	customHTTP := &http.Client{Timeout: 5 * time.Second}
	client := NewUpstreamClient(ts.URL, WithUpstreamHTTPClient(customHTTP))
	require.NoError(t, client.Connect(context.Background()))
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
}

func TestUpstreamClient_ConnectAlreadyConnected(t *testing.T) {
	ts, _ := newTestMCPServer()
	defer ts.Close()

	client := NewUpstreamClient(ts.URL)
	defer client.Close()
	require.NoError(t, client.Connect(context.Background()))
	require.NoError(t, client.Connect(context.Background()))
}

func TestUpstreamClient_DialWhenClosed(t *testing.T) {
	var client *UpstreamClient
	server := mcp.NewServer(&mcp.Implementation{Name: "test-upstream", Version: "1.0"}, nil)
	actualHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server {
			if client != nil {
				_ = client.Close()
			}
			return server
		},
		&mcp.StreamableHTTPOptions{JSONResponse: false, Stateless: false},
	)
	ts := httptest.NewServer(actualHandler)
	defer ts.Close()

	client = NewUpstreamClient(ts.URL)
	err := client.dial(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream client closed")
}

func TestUpstreamClient_ListTools_DialFails(t *testing.T) {
	client := NewUpstreamClient("http://127.0.0.1:1", WithUpstreamLogger(zap.NewNop()))
	defer client.Close()
	_, err := client.ListTools(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream not connected")
}

func TestUpstreamClient_CallTool_DialFails(t *testing.T) {
	client := NewUpstreamClient("http://127.0.0.1:1", WithUpstreamLogger(zap.NewNop()))
	defer client.Close()
	_, err := client.CallTool(context.Background(), "some_tool", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream not connected")
}

func TestUpstreamClient_CallTool_DialOnDemandSuccess(t *testing.T) {
	ts, _ := newTestMCPServer()
	defer ts.Close()

	client := NewUpstreamClient(ts.URL)
	defer client.Close()
	res, err := client.CallTool(context.Background(), "get_services", nil)
	require.NoError(t, err)
	assert.False(t, res.IsError)
}

func TestUpstreamClient_IsSessionMissingError(t *testing.T) {
	assert.False(t, isSessionMissingError(nil))
	assert.True(t, isSessionMissingError(mcp.ErrSessionMissing))
	assert.True(t, isSessionMissingError(errors.New("Session Not Found: 123")))
	assert.False(t, isSessionMissingError(assert.AnError))
}

func TestUpstreamClient_CloseUnconnected(t *testing.T) {
	client := NewUpstreamClient("http://127.0.0.1:1")
	require.NoError(t, client.Close())
}

func TestUpstreamClient_OptionsCoverage(t *testing.T) {
	client := NewUpstreamClient("http://127.0.0.1:1",
		WithUpstreamRetryInterval(-1),
		WithUpstreamMaxRetryInterval(5*time.Second),
		WithUpstreamMaxRetryInterval(-1),
	)
	assert.Equal(t, 5*time.Second, client.maxRetryInterval)
}
