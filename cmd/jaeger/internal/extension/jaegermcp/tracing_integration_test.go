// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func TestTracingE2E_MetaPropagation(t *testing.T) {
	capture := newTraceCapture(t)
	_, addr := startTestServerWithTelemetry(t, nil, telsetFromCapture(capture))
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	rootCtx, rootSpan := capture.provider.Tracer("test").Start(ctx, "test-root")
	sc := rootSpan.SpanContext()
	rootSpan.End()

	meta := mcp.Meta{}
	requestMetaPropagator.Inject(rootCtx, &requestMetaCarrier{meta: meta})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Meta: meta,
		Name: "health",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	span := findSpanByName(t, capture, "tools/call health")
	assert.Equal(t, sc.TraceID(), span.SpanContext.TraceID(),
		"MCP middleware span should belong to the same trace as the client")
	assert.Equal(t, sc.SpanID(), span.Parent.SpanID(),
		"MCP middleware span should be a child of the client span")
}

func TestTracingE2E_NonToolMethod(t *testing.T) {
	capture := newTraceCapture(t)
	_, addr := startTestServerWithTelemetry(t, nil, telsetFromCapture(capture))
	connectMCPClient(t, addr)

	span := findSpanByName(t, capture, "initialize")
	assertHasStringAttribute(t, span.Attributes,
		string(otelsemconv.McpMethodName("").Key), "initialize")
}

func TestTracingE2E_ToolErrorPath(t *testing.T) {
	capture := newTraceCapture(t)
	_, addr := startTestServerWithTelemetry(t, nil, telsetFromCapture(capture))
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_traces",
		Arguments: map[string]any{"start_time_min": "-1h"},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)

	span := findSpanByName(t, capture, "tools/call search_traces")
	assertHasStringAttribute(t, span.Attributes,
		string(otelsemconv.ErrorType("").Key), errorTypeTool)
}

func telsetFromCapture(capture *traceCapture) component.TelemetrySettings {
	telset := componenttest.NewNopTelemetrySettings()
	telset.TracerProvider = capture.provider
	return telset
}

func findSpanByName(t *testing.T, capture *traceCapture, name string) tracetest.SpanStub {
	t.Helper()
	var found tracetest.SpanStub
	require.Eventually(t, func() bool {
		spans := capture.exporter.GetSpans()
		for i := range spans {
			if spans[i].Name == name {
				found = spans[i]
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "span %q not found", name)
	return found
}
