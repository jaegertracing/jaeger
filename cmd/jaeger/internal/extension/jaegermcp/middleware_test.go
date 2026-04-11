// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func TestTracingMiddlewareToolCallSuccess(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	result, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_services"))
	require.NoError(t, err)
	require.NotNil(t, result)

	spanData := capture.singleSpan(t)
	assert.Equal(t, mcpMethodToolsCall+" get_services", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIToolName("").Key), "get_services")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIOperationNameExecuteTool.Key), "execute_tool")
	assert.Equal(t, codes.Unset, spanData.Status.Code)
}

func TestTracingMiddlewareToolCallError(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	expectedErr := errors.New("trace not found")
	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, expectedErr
	})

	_, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_trace_topology"))
	require.ErrorIs(t, err, expectedErr)

	spanData := capture.singleSpan(t)
	assert.Equal(t, mcpMethodToolsCall+" get_trace_topology", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIToolName("").Key), "get_trace_topology")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIOperationNameExecuteTool.Key), "execute_tool")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, expectedErr.Error(), spanData.Status.Description)
}

func TestTracingMiddlewareToolCallGenericError(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	expectedErr := errors.New("storage backend unavailable")
	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, expectedErr
	})

	_, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("search_traces"))
	require.ErrorIs(t, err, expectedErr)

	spanData := capture.singleSpan(t)
	assert.Equal(t, mcpMethodToolsCall+" search_traces", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIToolName("").Key), "search_traces")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIOperationNameExecuteTool.Key), "execute_tool")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, expectedErr.Error(), spanData.Status.Description)
}

func TestTracingMiddlewareToolCallResultError(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		result := &mcp.CallToolResult{}
		result.SetError(errors.New("invalid pattern"))
		return result, nil
	})

	result, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_services"))
	require.NoError(t, err)
	require.NotNil(t, result)

	spanData := capture.singleSpan(t)
	assert.Equal(t, mcpMethodToolsCall+" get_services", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIToolName("").Key), "get_services")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIOperationNameExecuteTool.Key), "execute_tool")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.ErrorType("").Key), errorTypeTool)
	assert.Equal(t, codes.Unset, spanData.Status.Code)
}

func TestTracingMiddlewareTracesNonToolMethods(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	_, err := wrapped(context.Background(), "initialize", newToolCallRequest("get_services"))
	require.NoError(t, err)

	spans := capture.waitForSpanCount(t, 1)
	require.Len(t, spans, 1)
	assert.Equal(t, "initialize", spans[0].Name)
	assertHasStringAttribute(t, spans[0].Attributes, string(otelsemconv.McpMethodName("").Key), "initialize")
}

func TestTracingMiddlewareNonToolMethodError(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	expectedErr := errors.New("initialize failed")
	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, expectedErr
	})

	_, err := wrapped(context.Background(), "initialize", nil)
	require.ErrorIs(t, err, expectedErr)

	spanData := capture.singleSpan(t)
	assert.Equal(t, "initialize", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.McpMethodName("").Key), "initialize")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, expectedErr.Error(), spanData.Status.Description)
}

func TestTracingMiddlewareCreatesChildSpanWhenParentExists(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

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
		if span.Name == mcpMethodToolsCall+" health" {
			childSpan = span
			foundChild = true
			break
		}
	}
	require.True(t, foundChild, "mcp tool child span should be present")
	assert.Equal(t, parentSpanID, childSpan.Parent.SpanID())
	assert.Equal(t, parentTraceID, childSpan.SpanContext.TraceID())
}

func TestTracingMiddlewareToolCallResultErrorWithoutConcreteError(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{IsError: true}, nil
	})

	_, err := wrapped(context.Background(), mcpMethodToolsCall, newToolCallRequest("get_services"))
	require.NoError(t, err)

	spanData := capture.singleSpan(t)
	assert.Equal(t, mcpMethodToolsCall+" get_services", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIToolName("").Key), "get_services")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.GenAIOperationNameExecuteTool.Key), "execute_tool")
	assertHasStringAttribute(t, spanData.Attributes, string(otelsemconv.ErrorType("").Key), errorTypeTool)
	assert.Equal(t, codes.Unset, spanData.Status.Code)
}

func TestToolNameFromRequest(t *testing.T) {
	req := newToolCallRequest("search_traces")
	assert.Equal(t, "search_traces", toolNameFromRequest(mcpMethodToolsCall, req))
	assert.Empty(t, toolNameFromRequest("initialize", req))
	assert.Empty(t, toolNameFromRequest(mcpMethodToolsCall, nil))
}

func TestToolNameFromRequestWrongParams(t *testing.T) {
	req := &mcp.ServerRequest[*mcp.InitializeParams]{
		Params: &mcp.InitializeParams{ProtocolVersion: "2025-03-26"},
	}
	assert.Empty(t, toolNameFromRequest(mcpMethodToolsCall, req))
}

func TestToolNameFromRequestNilParams(t *testing.T) {
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{}
	assert.Empty(t, toolNameFromRequest(mcpMethodToolsCall, req))
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

func chainMiddleware(middlewares ...mcp.Middleware) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

func TestSessionIDFromRequestNilCases(t *testing.T) {
	assert.Empty(t, sessionIDFromRequest(nil))

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Session: nil,
		Params:  &mcp.CallToolParamsRaw{Name: "health"},
	}
	assert.Empty(t, sessionIDFromRequest(req))

	clientReq := &mcp.ClientRequest[*mcp.CallToolParamsRaw]{
		Session: (*mcp.ClientSession)(nil),
		Params:  &mcp.CallToolParamsRaw{Name: "health"},
	}
	assert.Empty(t, sessionIDFromRequest(clientReq))

	serverReq := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Session: (*mcp.ServerSession)(nil),
		Params:  &mcp.CallToolParamsRaw{Name: "health"},
	}
	assert.Empty(t, sessionIDFromRequest(serverReq))
}

func TestSessionIDFromRequestWithNonNilSession(t *testing.T) {
	clientReq := &mcp.ClientRequest[*mcp.CallToolParamsRaw]{
		Session: &mcp.ClientSession{},
		Params:  &mcp.CallToolParamsRaw{Name: "health"},
	}
	assert.Empty(t, sessionIDFromRequest(clientReq))

	serverReq := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Session: &mcp.ServerSession{},
		Params:  &mcp.CallToolParamsRaw{Name: "health"},
	}
	assert.Empty(t, sessionIDFromRequest(serverReq))
}

func TestIsNilSession(t *testing.T) {
	var nilSession mcp.Session
	assert.True(t, isNilSession(nilSession))
	assert.True(t, isNilSession((*mcp.ServerSession)(nil)))
	assert.True(t, isNilSession((*mcp.ClientSession)(nil)))
	assert.False(t, isNilSession(&mcp.ServerSession{}))
	assert.False(t, isNilSession(&mcp.ClientSession{}))
}
