// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	baggageapi "go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	traceapi "go.opentelemetry.io/otel/trace"

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

func TestTracingMiddlewareUsesTraceContextFromRequestMeta(t *testing.T) {
	capture := newTraceCapture(t)
	middleware := chainMiddleware(createTracingMiddleware(capture.provider))

	wrapped := middleware(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	parentCtx, parentSpan := capture.provider.Tracer("test-parent").Start(context.Background(), "http.request")
	parentSC := parentSpan.SpanContext()

	req := newToolCallRequestWithMeta("get_services", mcp.Meta{
		traceContextMetaTraceParent: fmt.Sprintf("00-%s-%s-01", parentSC.TraceID(), parentSC.SpanID()),
	})
	_, err := wrapped(parentCtx, mcpMethodToolsCall, req)
	require.NoError(t, err)

	parentSpan.End()

	spans := capture.waitForSpanCount(t, 2)
	var childSpan tracetest.SpanStub
	foundChild := false
	for _, span := range spans {
		if span.Name == mcpMethodToolsCall+" get_services" {
			childSpan = span
			foundChild = true
			break
		}
	}
	require.True(t, foundChild, "mcp tool child span should be present")
	assert.Equal(t, parentSC.TraceID(), childSpan.SpanContext.TraceID())
	assert.Equal(t, parentSC.SpanID(), childSpan.Parent.SpanID())
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

func TestContextWithRequestMetaTraceContextNilCases(t *testing.T) {
	assert.NotPanics(t, func() {
		ctx := contextWithRequestMetaTraceContext(context.Background(), nil)
		assert.NotNil(t, ctx)
	})

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{}
	assert.NotPanics(t, func() {
		ctx := contextWithRequestMetaTraceContext(context.Background(), req)
		assert.NotNil(t, ctx)
	})
}

func TestContextWithRequestMetaTraceContextExtractsBaggage(t *testing.T) {
	req := newToolCallRequestWithMeta("health", mcp.Meta{
		traceContextMetaBaggage: "tenant.id=acme",
	})

	ctx := contextWithRequestMetaTraceContext(context.Background(), req)

	bag := baggageapi.FromContext(ctx)
	member := bag.Member("tenant.id")
	assert.Equal(t, "acme", member.Value())
	assert.False(t, traceapi.SpanContextFromContext(ctx).IsValid())
}

func TestContextWithRequestMetaTraceContextReplacesExistingBaggage(t *testing.T) {
	baseBag, err := baggageapi.Parse("tenant.id=acme,region=us-west")
	require.NoError(t, err)
	baseCtx := baggageapi.ContextWithBaggage(context.Background(), baseBag)

	req := newToolCallRequestWithMeta("health", mcp.Meta{
		traceContextMetaBaggage: "env=prod",
	})

	ctx := contextWithRequestMetaTraceContext(baseCtx, req)
	bag := baggageapi.FromContext(ctx)

	assert.Empty(t, bag.Member("tenant.id").Value())
	assert.Empty(t, bag.Member("region").Value())
	assert.Equal(t, "prod", bag.Member("env").Value())
}

func TestContextWithRequestMetaTraceContextIgnoresInvalidTraceparent(t *testing.T) {
	traceID, err := traceapi.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := traceapi.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	parentSC := traceapi.NewSpanContext(traceapi.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	})
	parentCtx := traceapi.ContextWithRemoteSpanContext(context.Background(), parentSC)

	req := newToolCallRequestWithMeta("health", mcp.Meta{
		traceContextMetaTraceParent: "invalid-traceparent",
	})
	ctx := contextWithRequestMetaTraceContext(parentCtx, req)

	assert.Equal(t, parentSC.TraceID(), traceapi.SpanContextFromContext(ctx).TraceID())
	assert.Equal(t, parentSC.SpanID(), traceapi.SpanContextFromContext(ctx).SpanID())
}

func newToolCallRequest(toolName string) *mcp.ServerRequest[*mcp.CallToolParamsRaw] {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: toolName},
	}
}

func newToolCallRequestWithMeta(toolName string, meta mcp.Meta) *mcp.ServerRequest[*mcp.CallToolParamsRaw] {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{
			Meta: meta,
			Name: toolName,
		},
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

func TestIsNil(t *testing.T) {
	var nilMap map[string]any
	var nilSlice []string
	var nilFunc func()
	var nilChan chan int
	var nilInterface any
	var nilSession mcp.Session
	typedNilMapInInterface := any(nilMap)

	assert.True(t, isNil(nil))
	assert.True(t, isNil(nilMap))
	assert.True(t, isNil(nilSlice))
	assert.True(t, isNil(nilFunc))
	assert.True(t, isNil(nilChan))
	assert.True(t, isNil(nilInterface))
	assert.True(t, isNil(nilSession))
	assert.True(t, isNil((*mcp.ServerSession)(nil)))
	assert.True(t, isNil((*mcp.ClientSession)(nil)))
	assert.True(t, isNil(typedNilMapInInterface))

	assert.False(t, isNil(map[string]any{}))
	assert.False(t, isNil([]string{}))
	assert.False(t, isNil(func() {}))
	assert.False(t, isNil(make(chan int)))
	assert.False(t, isNil(&mcp.ServerSession{}))
	assert.False(t, isNil(&mcp.ClientSession{}))
	assert.False(t, isNil(42))
}

func TestRequestMetaCarrier(t *testing.T) {
	meta := mcp.Meta{
		traceContextMetaTraceParent: "trace-parent",
		traceContextMetaBaggage:     "tenant.id=acme",
		traceContextMetaTraceState:  "",
		"other":                     "ignored",
	}
	carrier := &requestMetaCarrier{meta: meta}

	assert.Equal(t, "trace-parent", carrier.Get(traceContextMetaTraceParent))
	assert.Equal(t, "ignored", carrier.Get("other"))
	assert.Empty(t, carrier.Get("missing"))
	assert.ElementsMatch(t,
		[]string{traceContextMetaTraceParent, traceContextMetaTraceState, traceContextMetaBaggage, "other"},
		carrier.Keys(),
	)

	carrier.Set(traceContextMetaTraceState, "rojo=1")
	assert.Equal(t, "rojo=1", meta[traceContextMetaTraceState])
	assert.ElementsMatch(t,
		[]string{traceContextMetaTraceParent, traceContextMetaTraceState, traceContextMetaBaggage, "other"},
		carrier.Keys(),
	)

	nilMetaCarrier := &requestMetaCarrier{}
	assert.NotPanics(t, func() {
		nilMetaCarrier.Set(traceContextMetaTraceParent, "trace-parent")
	})
	assert.Equal(t, "trace-parent", nilMetaCarrier.Get(traceContextMetaTraceParent))
}
