// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// TODO: Add trace propagation test once #8361 (_meta extraction) lands.
// HTTP-level traceparent doesn't reach MCP middleware spans because the SDK's
// StreamableClientTransport dispatches handlers in the session context, not the
// per-request context. _meta propagation via CallToolParams.Meta bridges this gap.

func TestTracingE2E_NonToolMethod(t *testing.T) {
	capture := newTraceCapture(t)
	_, addr := startTestServerWithTelemetry(t, nil, telsetFromCapture(capture))
	connectTracedMCPClient(t, addr)

	span := findSpanByName(t, capture, "initialize")
	assertHasStringAttribute(t, span.Attributes,
		string(otelsemconv.McpMethodName("").Key), "initialize")
}

func TestTracingE2E_ToolErrorPath(t *testing.T) {
	capture := newTraceCapture(t)
	_, addr := startTestServerWithTelemetry(t, nil, telsetFromCapture(capture))
	session := connectTracedMCPClient(t, addr)

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

func connectTracedMCPClient(t *testing.T, addr string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "tracing-test", Version: "1.0.0"},
		nil,
	)

	transport := &mcp.StreamableClientTransport{
		Endpoint:   fmt.Sprintf("http://%s/mcp", addr),
		HTTPClient: http.DefaultClient,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })

	return session
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
