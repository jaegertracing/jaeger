// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// sendMCPRequest sends a JSON-RPC request to the MCP endpoint and returns
// the session ID from the response.
func sendMCPRequest(t *testing.T, addr, sessionID, body string) string {
	t.Helper()
	httpReq, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://%s/mcp", addr),
		bytes.NewReader([]byte(body)),
	)
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		return sid
	}
	return sessionID
}

// deleteMCPSession cleans up an MCP session to avoid goroutine leaks.
func deleteMCPSession(t *testing.T, addr, sessionID string) {
	t.Helper()
	if sessionID == "" {
		return
	}
	delReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/mcp", addr), http.NoBody)
	require.NoError(t, err)
	delReq.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	resp.Body.Close()
}

func TestLoggingMiddleware(t *testing.T) {
	zapCore, logs := observer.New(zapcore.DebugLevel)
	_, addr := startTestServerWithQueryService(t, nil, zap.New(zapCore))

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	sessionID := sendMCPRequest(t, addr, "", initReq)
	t.Cleanup(func() { deleteMCPSession(t, addr, sessionID) })

	requestLogs := logs.FilterMessage("MCP request").All()
	require.Len(t, requestLogs, 1)
	reqFields := requestLogs[0].ContextMap()
	assert.Equal(t, "initialize", reqFields["method"])
	assert.NotEmpty(t, reqFields["session_id"])
	assert.NotContains(t, reqFields, "tool", "initialize should not have a tool field")

	responseLogs := logs.FilterMessage("MCP response").All()
	require.Len(t, responseLogs, 1)
	respFields := responseLogs[0].ContextMap()
	assert.Equal(t, "initialize", respFields["method"])
	assert.NotEmpty(t, respFields["session_id"])
	assert.Contains(t, respFields, "duration")
	assert.NotContains(t, respFields, "tool", "initialize should not have a tool field")
}

func TestLoggingMiddlewareToolName(t *testing.T) {
	zapCore, logs := observer.New(zapcore.DebugLevel)
	_, addr := startTestServerWithQueryService(t, nil, zap.New(zapCore))

	// Initialize session first
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	sessionID := sendMCPRequest(t, addr, "", initReq)
	require.NotEmpty(t, sessionID)
	t.Cleanup(func() { deleteMCPSession(t, addr, sessionID) })

	// Call the health tool
	toolReq := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"health","arguments":{}}}`
	sendMCPRequest(t, addr, sessionID, toolReq)

	// Filter to tools/call request log and verify tool name
	toolCallLogs := logs.FilterMessage("MCP request").
		FilterField(zap.String("method", "tools/call")).All()
	require.Len(t, toolCallLogs, 1)
	assert.Equal(t, "health", toolCallLogs[0].ContextMap()["tool"])

	// Filter to tools/call response log and verify tool name
	toolRespLogs := logs.FilterMessage("MCP response").
		FilterField(zap.String("method", "tools/call")).All()
	require.Len(t, toolRespLogs, 1)
	assert.Equal(t, "health", toolRespLogs[0].ContextMap()["tool"])
	assert.Contains(t, toolRespLogs[0].ContextMap(), "duration")
}
