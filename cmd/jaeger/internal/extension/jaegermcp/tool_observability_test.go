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
)

func TestInstrumentToolSuccess(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(capture.provider)

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
}

func TestInstrumentToolError(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(capture.provider)

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
}

func TestInstrumentToolErrorFromResultObject(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(capture.provider)

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
}

func TestInstrumentToolErrorFromResultObjectWithoutErrorValue(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(capture.provider)

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
	capture := newTraceCapture(t)
	obs := newToolObservability(capture.provider)

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
}

func TestInstrumentToolCreatesChildSpanWhenParentExists(t *testing.T) {
	capture := newTraceCapture(t)
	obs := newToolObservability(capture.provider)

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

func TestNewToolObservabilityDefaults(t *testing.T) {
	obs := newToolObservability(nil)
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
