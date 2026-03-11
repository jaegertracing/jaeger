// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestInstrumentToolSuccess(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	metricsFactory := metricstest.NewFactory(0)
	obs := newToolObservability(logger, metricsFactory)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ types.GetSpanNamesInput) (*mcp.CallToolResult, types.GetSpanNamesOutput, error) {
		return nil, types.GetSpanNamesOutput{
			SpanNames: []types.SpanNameInfo{
				{Name: "GET /checkout", SpanKind: "SERVER"},
				{Name: "POST /checkout", SpanKind: "SERVER"},
			},
		}, nil
	}
	wrapped := instrumentTool(obs, "get_span_names", handler)

	input := types.GetSpanNamesInput{ServiceName: "checkout", Limit: 25}
	labeler := &otelhttp.Labeler{}
	ctx := otelhttp.ContextWithLabeler(context.Background(), labeler)

	result, output, err := wrapped(ctx, nil, input)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Len(t, output.SpanNames, 2)

	metricsFactory.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{
			Name:  "response_items",
			Tags:  map[string]string{"tool_name": "get_span_names", "status": "ok"},
			Value: 2,
		},
	)

	assertHasStringAttribute(t, labeler.Get(), "mcp.tool_name", "get_span_names")
	assertHasStringAttribute(t, labeler.Get(), "mcp.status", "ok")

	doneLogs := observed.FilterMessage("MCP tool invocation completed").All()
	require.Len(t, doneLogs, 1)
	doneContext := doneLogs[0].ContextMap()
	assert.Equal(t, "get_span_names", doneContext["tool_name"])
	assert.Equal(t, "ok", doneContext["status"])
	assert.EqualValues(t, 2, doneContext["result_count"])
	assert.Equal(t, "checkout", doneContext["service_name"])
}

func TestInstrumentToolError(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	metricsFactory := metricstest.NewFactory(0)
	obs := newToolObservability(logger, metricsFactory)

	expectedErr := errors.New("trace not found")
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ types.GetTraceTopologyInput) (*mcp.CallToolResult, types.GetTraceTopologyOutput, error) {
		return nil, types.GetTraceTopologyOutput{}, expectedErr
	}
	wrapped := instrumentTool(obs, "get_trace_topology", handler)

	labeler := &otelhttp.Labeler{}
	ctx := otelhttp.ContextWithLabeler(context.Background(), labeler)

	_, _, err := wrapped(ctx, nil, types.GetTraceTopologyInput{TraceID: "deadbeef"})
	require.ErrorIs(t, err, expectedErr)

	assertHasStringAttribute(t, labeler.Get(), "mcp.tool_name", "get_trace_topology")
	assertHasStringAttribute(t, labeler.Get(), "mcp.status", "not_found")

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.WarnLevel, failedLogs[0].Level)
	failedContext := failedLogs[0].ContextMap()
	assert.Equal(t, "get_trace_topology", failedContext["tool_name"])
	assert.Equal(t, "not_found", failedContext["status"])
	assert.Equal(t, "deadbeef", failedContext["trace_id"])
	_, hasErrorField := failedContext["error"]
	assert.True(t, hasErrorField)
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

func TestInferResultCountFromTopologyOutput(t *testing.T) {
	root := &types.SpanNode{
		SpanID: "root",
		Children: []*types.SpanNode{
			{SpanID: "child-a"},
			{SpanID: "child-b", Children: []*types.SpanNode{{SpanID: "grandchild"}}},
		},
	}
	orphans := []*types.SpanNode{{SpanID: "orphan"}}

	count, ok := inferResultCount(types.GetTraceTopologyOutput{RootSpan: root, Orphans: orphans})
	require.True(t, ok)
	assert.Equal(t, 5, count)
}

func TestInstrumentToolNilObservabilityReturnsOriginalHandler(t *testing.T) {
	handler := func(_ context.Context, _ *mcp.CallToolRequest, input types.GetServicesInput) (*mcp.CallToolResult, types.GetServicesOutput, error) {
		return nil, types.GetServicesOutput{Services: []string{input.Pattern}}, nil
	}
	wrapped := instrumentTool[types.GetServicesInput, types.GetServicesOutput](nil, "get_services", handler)

	_, output, err := wrapped(context.Background(), nil, types.GetServicesInput{Pattern: "checkout"})
	require.NoError(t, err)
	assert.Equal(t, []string{"checkout"}, output.Services)
}

func TestInstrumentToolErrorFromResultObject(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	metricsFactory := metricstest.NewFactory(0)
	obs := newToolObservability(logger, metricsFactory)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		result := &mcp.CallToolResult{}
		result.SetError(errors.New("invalid pattern"))
		return result, struct{}{}, nil
	}
	wrapped := instrumentTool(obs, "get_services", handler)

	labeler := &otelhttp.Labeler{}
	ctx := otelhttp.ContextWithLabeler(context.Background(), labeler)

	result, _, err := wrapped(ctx, nil, struct{}{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	assertHasStringAttribute(t, labeler.Get(), "mcp.tool_name", "get_services")
	assertHasStringAttribute(t, labeler.Get(), "mcp.status", "invalid_argument")

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.WarnLevel, failedLogs[0].Level)
	assert.Equal(t, "invalid_argument", failedLogs[0].ContextMap()["status"])
}

func TestInstrumentToolPanic(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	metricsFactory := metricstest.NewFactory(0)
	obs := newToolObservability(logger, metricsFactory)

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		panic("boom")
	}
	wrapped := instrumentTool(obs, "health", handler)

	labeler := &otelhttp.Labeler{}
	ctx := otelhttp.ContextWithLabeler(context.Background(), labeler)

	result, _, err := wrapped(ctx, nil, struct{}{})
	require.ErrorIs(t, err, errToolHandlerPanic)
	require.Nil(t, result)

	assertHasStringAttribute(t, labeler.Get(), "mcp.tool_name", "health")
	assertHasStringAttribute(t, labeler.Get(), "mcp.status", "error")

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
	assert.Equal(t, zapcore.ErrorLevel, failedLogs[0].Level)
	failedContext := failedLogs[0].ContextMap()
	assert.Equal(t, "health", failedContext["tool_name"])
	assert.Equal(t, "error", failedContext["status"])
	_, hasPanicField := failedContext["panic"]
	assert.True(t, hasPanicField)
}

func TestNewToolObservabilityDefaults(t *testing.T) {
	obs := newToolObservability(nil, nil)
	require.NotNil(t, obs.logger)
	require.NotNil(t, obs.factory)
}

func TestToolMetricsStatusFallback(t *testing.T) {
	obs := newToolObservability(zap.NewNop(), metricstest.NewFactory(0))
	metricsForTool := obs.metricsForTool("health")
	require.NotNil(t, metricsForTool.status(toolStatusError))
	assert.NotNil(t, metricsForTool.status("not-a-valid-status"))
}

func TestSummarizeRequestKnownInputs(t *testing.T) {
	summary := summarizeRequest(types.GetSpanNamesInput{ServiceName: "checkout", Limit: 25})
	assert.Equal(t, "checkout", summary.serviceName)
	assert.True(t, summary.hasRequestedLimit)
	assert.Equal(t, 25, summary.requestedLimit)

	summaryWithSpanIDs := summarizeRequest(types.GetSpanDetailsInput{TraceID: "abc", SpanIDs: []string{"1", "2", "3"}})
	assert.Equal(t, "abc", summaryWithSpanIDs.traceID)
	assert.True(t, summaryWithSpanIDs.hasRequestedLimit)
	assert.Equal(t, 3, summaryWithSpanIDs.requestedLimit)
}

func TestInferResultCountNonSupportedOutput(t *testing.T) {
	count, ok := inferResultCount("invalid")
	assert.False(t, ok)
	assert.Zero(t, count)
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
