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
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestInstrumentToolSuccess(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.New(core), capture.provider)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	}
	wrapped := instrumentTool(obs, "get_services", handler)

	result, _, err := wrapped(context.Background(), nil, struct{}{})

	require.NoError(t, err)
	require.Nil(t, result)

	spanData := capture.singleSpan(t)
	assert.Equal(t, "mcp.tool.get_services", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_services")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", "ok")
	assert.Equal(t, codes.Unset, spanData.Status.Code)

	doneLogs := observed.FilterMessage("MCP tool invocation completed").All()
	require.Len(t, doneLogs, 1)
	doneContext := doneLogs[0].ContextMap()
	assert.Equal(t, "get_services", doneContext["tool_name"])
	assert.Equal(t, "ok", doneContext["status"])
}

func TestInstrumentToolError(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.New(core), capture.provider)

	expectedErr := errors.New("trace not found")
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, expectedErr
	}
	wrapped := instrumentTool(obs, "get_trace_topology", handler)

	_, _, err := wrapped(context.Background(), nil, struct{}{})
	require.ErrorIs(t, err, expectedErr)

	spanData := capture.singleSpan(t)
	assert.Equal(t, "mcp.tool.get_trace_topology", spanData.Name)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_trace_topology")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", "not_found")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, expectedErr.Error(), spanData.Status.Description)

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.WarnLevel, failedLogs[0].Level)
	assert.Equal(t, "not_found", failedLogs[0].ContextMap()["status"])
}

func TestInstrumentToolErrorFromResultObject(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.New(core), capture.provider)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		result := &mcp.CallToolResult{}
		result.SetError(errors.New("invalid pattern"))
		return result, struct{}{}, nil
	}
	wrapped := instrumentTool(obs, "get_services", handler)

	result, _, err := wrapped(context.Background(), nil, struct{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	spanData := capture.singleSpan(t)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_services")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", "invalid_argument")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, "invalid pattern", spanData.Status.Description)

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.WarnLevel, failedLogs[0].Level)
}

func TestInstrumentToolErrorFromResultObjectWithoutErrorValue(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.NewNop(), capture.provider)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{IsError: true}, struct{}{}, nil
	}
	wrapped := instrumentTool(obs, "get_services", handler)

	_, _, err := wrapped(context.Background(), nil, struct{}{})
	require.NoError(t, err)

	spanData := capture.singleSpan(t)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "get_services")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", "error")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, errToolResultMarkedError.Error(), spanData.Status.Description)
}

func TestInstrumentToolGenericError(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.New(core), capture.provider)

	expectedErr := errors.New("storage backend unavailable")
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, expectedErr
	}
	wrapped := instrumentTool(obs, "search_traces", handler)

	_, _, err := wrapped(context.Background(), nil, struct{}{})
	require.ErrorIs(t, err, expectedErr)

	spanData := capture.singleSpan(t)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "search_traces")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", "error")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, expectedErr.Error(), spanData.Status.Description)

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.ErrorLevel, failedLogs[0].Level)
}

func TestInstrumentToolPanic(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.New(core), capture.provider)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		panic("boom")
	}
	wrapped := instrumentTool(obs, "health", handler)

	result, _, err := wrapped(context.Background(), nil, struct{}{})

	require.ErrorIs(t, err, errToolHandlerPanic)
	require.Nil(t, result)

	spanData := capture.singleSpan(t)
	assertHasStringAttribute(t, spanData.Attributes, "mcp.tool.name", "health")
	assertHasStringAttribute(t, spanData.Attributes, "mcp.status", "error")
	assert.Equal(t, codes.Error, spanData.Status.Code)
	assert.Equal(t, errToolHandlerPanic.Error(), spanData.Status.Description)

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.ErrorLevel, failedLogs[0].Level)
	_, hasPanicField := failedLogs[0].ContextMap()["panic"]
	assert.True(t, hasPanicField)
}

func TestInstrumentToolCreatesChildSpanWhenParentExists(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(zap.NewNop(), capture.provider)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	}
	wrapped := instrumentTool(obs, "health", handler)

	parentCtx, parentSpan := capture.provider.Tracer("test-parent").Start(context.Background(), "http.request")
	parentSpanID := parentSpan.SpanContext().SpanID()
	parentTraceID := parentSpan.SpanContext().TraceID()

	_, _, err := wrapped(parentCtx, nil, struct{}{})
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

func TestInstrumentToolNilObservabilityReturnsOriginalHandler(t *testing.T) {
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, string, error) {
		return nil, "ok", nil
	}
	wrapped := instrumentTool[struct{}, string](nil, "health", handler)

	_, output, err := wrapped(context.Background(), nil, struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "ok", output)
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

func TestLogFailureErrorLevel(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	obs := newToolObservability(zap.New(core), nil)
	obs.logFailure(toolStatusError, zap.String("tool_name", "x"))
	entries := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, entries, 1)
	assert.Equal(t, zapcore.ErrorLevel, entries[0].Level)
}

func TestNewToolObservabilityDefaults(t *testing.T) {
	obs := newToolObservability(nil, nil)
	require.NotNil(t, obs.logger)
	require.NotNil(t, obs.tracer)
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
