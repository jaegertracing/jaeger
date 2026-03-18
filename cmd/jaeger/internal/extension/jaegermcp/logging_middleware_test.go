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

func TestLoggingMiddleware(t *testing.T) {
	zapCore, logs := observer.New(zapcore.DebugLevel)
	_, addr := startTestServerWithQueryService(t, nil, zap.New(zapCore))

	// Send an MCP initialize request, which passes through the logging middleware.
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	httpReq, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://%s/mcp", addr),
		bytes.NewReader([]byte(initReq)),
	)
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Clean up the MCP session to avoid goroutine leaks.
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID != "" {
		delReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/mcp", addr), http.NoBody)
		require.NoError(t, err)
		delReq.Header.Set("Mcp-Session-Id", sessionID)
		resp2, err := http.DefaultClient.Do(delReq)
		require.NoError(t, err)
		resp2.Body.Close()
	}

	// The middleware should have emitted one "MCP request" and one "MCP response" entry.
	requestLogs := logs.FilterMessage("MCP request").All()
	require.Len(t, requestLogs, 1)
	reqFields := requestLogs[0].ContextMap()
	assert.Equal(t, "initialize", reqFields["method"])
	assert.NotEmpty(t, reqFields["session_id"])

	responseLogs := logs.FilterMessage("MCP response").All()
	require.Len(t, responseLogs, 1)
	respFields := responseLogs[0].ContextMap()
	assert.Equal(t, "initialize", respFields["method"])
	assert.NotEmpty(t, respFields["session_id"])
	assert.Contains(t, respFields, "duration")
}
