// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/metrics"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// TestToolErrorSpanStatusVsMetricStatus drives a real mcptools server, real
// handlers, both middlewares, in-memory transport, and calls get_trace_topology
// for a trace the store does not have. The handler returns
// errors.New("trace not found"), which the MCP SDK converts into
// CallToolResult{IsError: true} with a nil Go error.
//
// Tracing and metrics must agree: jaeger.mcp.tool.calls{status="error"} and
// span Status.Code = Error.
func TestToolErrorSpanStatusVsMetricStatus(t *testing.T) {
	traceCap := newTraceCapture(t)
	metricCap := newMetricsCapture(t)

	reader := &tracestoremocks.Reader{}
	reader.EXPECT().GetTraces(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(func([]ptrace.Traces, error) bool) {}
		},
	)
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})

	server := NewServer(telemetry.Settings{
		Logger:         zap.NewNop(),
		Metrics:        metrics.NullFactory,
		MeterProvider:  metricCap.provider,
		TracerProvider: traceCap.provider,
	}, svc, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "repro-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_trace_topology",
		Arguments: map[string]any{"trace_id": "00000000000000000000000000000abc"},
	})
	require.NoError(t, err, "no transport-level error: the failure is carried in the result")
	require.True(t, res.IsError, "the tool call failed")

	require.NoError(t, traceCap.provider.ForceFlush(ctx))
	spans := traceCap.exporter.GetSpans()
	require.NotEmpty(t, spans)

	toolSpanName := mcpMethodToolsCall + " get_trace_topology"
	var toolSpan tracetest.SpanStub
	found := false
	for _, s := range spans {
		if s.Name == toolSpanName {
			toolSpan = s
			found = true
			break
		}
	}
	require.True(t, found, "expected a span named %q", toolSpanName)

	rm := metricCap.collect(t)
	counter := findMetricDataPoint[metricdata.Sum[int64]](t, rm, "jaeger.mcp.tool.calls")

	var toolCallStatus string
	for _, p := range counter.DataPoints {
		status, _ := p.Attributes.Value("status")
		toolName, _ := p.Attributes.Value("gen_ai.tool.name")
		if toolName.AsString() == "get_trace_topology" {
			toolCallStatus = status.AsString()
			break
		}
	}

	assert.Equal(t, metricStatusError, toolCallStatus,
		"metrics middleware classifies the failed call as an error")
	assert.Equal(t, codes.Error, toolSpan.Status.Code,
		"tracing middleware must mark the same failed call as Error")
	assertHasStringAttribute(t, toolSpan.Attributes,
		string(otelsemconv.ErrorType("").Key), errorTypeTool)
}
