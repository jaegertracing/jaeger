// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggingMiddlewareTracesToolCallSuccess(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	middleware := createLoggingMiddleware(zap.New(core), capture.provider)

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	result, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_services"))
	require.NoError(t, err)
	require.NotNil(t, result)

	spanData := capture.singleSpan(t)
	assert.Equal(t, "mcp.tool.get_services", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_services")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", toolStatusOK)
	assert.Equal(t, codes.Unset, spanData.Status.Code)

	responseLogs := observed.FilterMessage("MCP response").All()
	require.Len(t, responseLogs, 1)
	assert.Equal(t, zapcore.InfoLevel, responseLogs[0].Level)
	assert.Equal(t, "get_services", responseLogs[0].ContextMap()["tool_name"])
	assert.Equal(t, toolStatusOK, responseLogs[0].ContextMap()["status"])
}

func TestLoggingMiddlewareTracesToolCallError(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	middleware := createLoggingMiddleware(zap.New(core), capture.provider)

	expectedErr := errors.New("trace not found")
	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, expectedErr
	})

	_, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_trace_topology"))
	require.ErrorIs(t, err, expectedErr)

	spanData := capture.singleSpan(t)
	assert.Equal(t, "mcp.tool.get_trace_topology", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_trace_topology")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", toolStatusNotFound)
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, expectedErr.Error(), spanData.Status.Description)

	responseLogs := observed.FilterMessage("MCP response").All()
	require.Len(t, responseLogs, 1)
	assert.Equal(t, zapcore.WarnLevel, responseLogs[0].Level)
	assert.Equal(t, toolStatusNotFound, responseLogs[0].ContextMap()["status"])
}

func TestLoggingMiddlewareTracesToolCallResultError(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	middleware := createLoggingMiddleware(zap.New(core), capture.provider)

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		result := &mcp.CallToolResult{}
		result.SetError(errors.New("invalid pattern"))
		return result, nil
	})

	result, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_services"))
	require.NoError(t, err)
	require.NotNil(t, result)

	spanData := capture.singleSpan(t)
	assert.Equal(t, "mcp.tool.get_services", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_services")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", toolStatusInvalidArgument)
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, "invalid pattern", spanData.Status.Description)

	responseLogs := observed.FilterMessage("MCP response").All()
	require.Len(t, responseLogs, 1)
	assert.Equal(t, zapcore.WarnLevel, responseLogs[0].Level)
	assert.Equal(t, toolStatusInvalidArgument, responseLogs[0].ContextMap()["status"])
}

func TestLoggingMiddlewareDoesNotTraceNonToolMethods(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := createLoggingMiddleware(zap.NewNop(), capture.provider)

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	_, err := wrapped(context.Background(), "initialize", newToolCallRequest("get_services"))
	require.NoError(t, err)

	spans := capture.waitForSpanCount(t, 0)
	assert.Empty(t, spans)
}

func TestLoggingMiddlewareCreatesChildSpanWhenParentExists(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := createLoggingMiddleware(zap.NewNop(), capture.provider)

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	parentCtx, parentSpan := capture.provider.Tracer("test-parent").Start(context.Background(), "http.request")
	parentSpanID := parentSpan.SpanContext().SpanID()
	parentTraceID := parentSpan.SpanContext().TraceID()

	_, err := wrapped(parentCtx, mcpMethodToolsCall, newToolCallRequest("health"))
	require.NoError(t, err)

	parentSpan.End()

	spans := capture.waitForSpanCount(t, 2)
	var childSpan tracetest.SpanStub
	foundChild := false
	for _, span := range spans {
		if span.Name == "mcp.tool.health" {
			childSpan = span
			foundChild = true
			break
		}
	}
	require.True(t, foundChild, "mcp tool child span should be present")
	assert.Equal(t, parentSpanID, childSpan.Parent.SpanID())
	assert.Equal(t, parentTraceID, childSpan.SpanContext.TraceID())
}

func TestToolNameFromRequest(t *testing.T) {
	req := newToolCallRequest("search_traces")
	assert.Equal(t, "search_traces", toolNameFromRequest(mcpMethodToolsCall, req))
	assert.Empty(t, toolNameFromRequest("initialize", req))
	assert.Empty(t, toolNameFromRequest(mcpMethodToolsCall, nil))
}

func TestNormalizeToolStatus(t *testing.T) {
	resultWithNotFound := &mcp.CallToolResult{}
	resultWithNotFound.SetError(errors.New("service not found"))

	tests := []struct {
		name   string
		err    error
		result *mcp.CallToolResult
		want   string
	}{
		{name: "ok", want: toolStatusOK},
		{name: "invalid argument", err: errors.New("service_name is required"), want: toolStatusInvalidArgument},
		{name: "not found", err: errors.New("trace not found"), want: toolStatusNotFound},
		{name: "generic error", err: errors.New("storage backend unavailable"), want: toolStatusError},
		{name: "result error not found", result: resultWithNotFound, want: toolStatusNotFound},
		{name: "result error generic", result: &mcp.CallToolResult{IsError: true}, want: toolStatusError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeToolStatus(tt.err, tt.result))
		})
	}
}

func TestObserveToolInSpanWithoutRecordingSpan(t *testing.T) {
	assert.NotPanics(t, func() {
		observeToolInSpan(trace.SpanFromContext(context.Background()), toolStatusOK, nil)
	})
}

func TestNewToolTracerDefaults(t *testing.T) {
	tracer := newToolTracer(nil)
	require.NotNil(t, tracer)
}

func newToolCallRequest(toolName string) *mcp.ServerRequest[*mcp.CallToolParamsRaw] {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: toolName},
	}
}

func assertHasStringAttribute(t *testing.T, attrs []attribute.KeyValue, key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.AsString() == value {
			return
		}
	}
	t.Fatalf("attribute %s=%s not found in %+v", key, value, attrs)
}

type traceCapture struct {
	provider *tracesdk.TracerProvider
	exporter *tracetest.InMemoryExporter
}

func newTraceCapture(t *testing.T) *traceCapture {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := tracesdk.NewTracerProvider(
		tracesdk.WithSyncer(exporter),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return &traceCapture{provider: provider, exporter: exporter}
}

func (c *traceCapture) singleSpan(t *testing.T) tracetest.SpanStub {
	t.Helper()
	spans := c.waitForSpanCount(t, 1)
	return spans[0]
}

func (c *traceCapture) waitForSpanCount(t *testing.T, want int) []tracetest.SpanStub {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(c.exporter.GetSpans()) == want
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, c.provider.ForceFlush(context.Background()))
	spans := c.exporter.GetSpans()
	require.Lenf(t, spans, want, "expected %d spans", want)
	return spans
}

func TestMiddlewareInitializeRequestLogging(t *testing.T) {
	zapCore, logs := observer.New(zapcore.DebugLevel)
	_, addr := startTestServerWithQueryService(t, nil, zap.New(zapCore))

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

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID != "" {
		delReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/mcp", addr), http.NoBody)
		require.NoError(t, err)
		delReq.Header.Set("Mcp-Session-Id", sessionID)
		resp2, err := http.DefaultClient.Do(delReq)
		require.NoError(t, err)
		resp2.Body.Close()
	}

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
