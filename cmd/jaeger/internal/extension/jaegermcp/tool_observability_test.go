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

func TestToolMetricsStatusFallback(t *testing.T) {
	obs := newToolObservability(zap.NewNop(), metricstest.NewFactory(0))
	first := obs.metricsForTool("health")
	second := obs.metricsForTool("health")
	require.Same(t, first, second)
	require.NotNil(t, first.status(toolStatusError))
	assert.NotNil(t, first.status("not-a-valid-status"))
}

func TestNewToolObservabilityDefaults(t *testing.T) {
	obs := newToolObservability(nil, nil)
	require.NotNil(t, obs.logger)
	require.NotNil(t, obs.factory)
}

func TestSummarizeRequestVariants(t *testing.T) {
	servicesIn := types.GetServicesInput{Limit: 5}
	spanNamesIn := types.GetSpanNamesInput{ServiceName: "checkout", Limit: 7}
	searchIn := types.SearchTracesInput{ServiceName: "frontend", SearchDepth: 9}
	topologyIn := types.GetTraceTopologyInput{TraceID: "t1", Depth: 2}
	spanDetailsIn := types.GetSpanDetailsInput{TraceID: "t2", SpanIDs: []string{"s1", "s2"}}
	traceErrorsIn := types.GetTraceErrorsInput{TraceID: "t3"}
	criticalPathIn := types.GetCriticalPathInput{TraceID: "t4"}

	tests := []struct {
		name  string
		input any
		want  requestSummary
	}{
		{name: "services value", input: servicesIn, want: requestSummary{requestedLimit: 5, hasRequestedLimit: true}},
		{name: "services ptr value", input: &servicesIn, want: requestSummary{requestedLimit: 5, hasRequestedLimit: true}},
		{name: "services ptr nil", input: (*types.GetServicesInput)(nil), want: requestSummary{}},
		{name: "span names value", input: spanNamesIn, want: requestSummary{serviceName: "checkout", requestedLimit: 7, hasRequestedLimit: true}},
		{name: "span names ptr value", input: &spanNamesIn, want: requestSummary{serviceName: "checkout", requestedLimit: 7, hasRequestedLimit: true}},
		{name: "span names ptr nil", input: (*types.GetSpanNamesInput)(nil), want: requestSummary{}},
		{name: "search value", input: searchIn, want: requestSummary{serviceName: "frontend", requestedLimit: 9, hasRequestedLimit: true}},
		{name: "search ptr value", input: &searchIn, want: requestSummary{serviceName: "frontend", requestedLimit: 9, hasRequestedLimit: true}},
		{name: "search ptr nil", input: (*types.SearchTracesInput)(nil), want: requestSummary{}},
		{name: "topology value", input: topologyIn, want: requestSummary{traceID: "t1", requestedLimit: 2, hasRequestedLimit: true}},
		{name: "topology ptr value", input: &topologyIn, want: requestSummary{traceID: "t1", requestedLimit: 2, hasRequestedLimit: true}},
		{name: "topology ptr nil", input: (*types.GetTraceTopologyInput)(nil), want: requestSummary{}},
		{name: "span details value", input: spanDetailsIn, want: requestSummary{traceID: "t2", requestedLimit: 2, hasRequestedLimit: true}},
		{name: "span details ptr value", input: &spanDetailsIn, want: requestSummary{traceID: "t2", requestedLimit: 2, hasRequestedLimit: true}},
		{name: "span details ptr nil", input: (*types.GetSpanDetailsInput)(nil), want: requestSummary{}},
		{name: "trace errors value", input: traceErrorsIn, want: requestSummary{traceID: "t3"}},
		{name: "trace errors ptr value", input: &traceErrorsIn, want: requestSummary{traceID: "t3"}},
		{name: "trace errors ptr nil", input: (*types.GetTraceErrorsInput)(nil), want: requestSummary{}},
		{name: "critical path value", input: criticalPathIn, want: requestSummary{traceID: "t4"}},
		{name: "critical path ptr value", input: &criticalPathIn, want: requestSummary{traceID: "t4"}},
		{name: "critical path ptr nil", input: (*types.GetCriticalPathInput)(nil), want: requestSummary{}},
		{name: "unsupported input", input: "invalid", want: requestSummary{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, summarizeRequest(tt.input))
		})
	}
}

func TestInferResultCountVariants(t *testing.T) {
	servicesOut := types.GetServicesOutput{Services: []string{"a", "b"}}
	spanNamesOut := types.GetSpanNamesOutput{SpanNames: []types.SpanNameInfo{{Name: "n1"}, {Name: "n2"}, {Name: "n3"}}}
	searchOut := types.SearchTracesOutput{Traces: []types.TraceSummary{{TraceID: "t1"}}}
	criticalOut := types.GetCriticalPathOutput{Segments: []types.CriticalPathSegment{{SpanID: "s1"}, {SpanID: "s2"}}}
	spanDetailsOut := types.GetSpanDetailsOutput{Spans: []types.SpanDetail{{SpanID: "s1"}}}
	traceErrorsOut := types.GetTraceErrorsOutput{Spans: []types.SpanDetail{{SpanID: "e1"}, {SpanID: "e2"}}}
	topologyOut := types.GetTraceTopologyOutput{
		RootSpan: types.SpanNode{SpanID: "root"},
		Orphans:  []types.SpanNode{{SpanID: "orphan"}},
	}

	tests := []struct {
		name   string
		output any
		count  int
		ok     bool
	}{
		{name: "services value", output: servicesOut, count: 2, ok: true},
		{name: "services ptr value", output: &servicesOut, count: 2, ok: true},
		{name: "services ptr nil", output: (*types.GetServicesOutput)(nil), count: 0, ok: false},
		{name: "span names value", output: spanNamesOut, count: 3, ok: true},
		{name: "span names ptr value", output: &spanNamesOut, count: 3, ok: true},
		{name: "span names ptr nil", output: (*types.GetSpanNamesOutput)(nil), count: 0, ok: false},
		{name: "search value", output: searchOut, count: 1, ok: true},
		{name: "search ptr value", output: &searchOut, count: 1, ok: true},
		{name: "search ptr nil", output: (*types.SearchTracesOutput)(nil), count: 0, ok: false},
		{name: "critical path value", output: criticalOut, count: 2, ok: true},
		{name: "critical path ptr value", output: &criticalOut, count: 2, ok: true},
		{name: "critical path ptr nil", output: (*types.GetCriticalPathOutput)(nil), count: 0, ok: false},
		{name: "span details value", output: spanDetailsOut, count: 1, ok: true},
		{name: "span details ptr value", output: &spanDetailsOut, count: 1, ok: true},
		{name: "span details ptr nil", output: (*types.GetSpanDetailsOutput)(nil), count: 0, ok: false},
		{name: "trace errors value", output: traceErrorsOut, count: 2, ok: true},
		{name: "trace errors ptr value", output: &traceErrorsOut, count: 2, ok: true},
		{name: "trace errors ptr nil", output: (*types.GetTraceErrorsOutput)(nil), count: 0, ok: false},
		{name: "topology value", output: topologyOut, count: 2, ok: true},
		{name: "topology ptr value", output: &topologyOut, count: 2, ok: true},
		{name: "topology ptr nil", output: (*types.GetTraceTopologyOutput)(nil), count: 0, ok: false},
		{name: "unsupported output", output: "invalid", count: 0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := inferResultCount(tt.output)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.count, got)
		})
	}
}

func TestCountSpanHelpers(t *testing.T) {
	node := &types.SpanNode{
		SpanID:   "root",
		Children: []*types.SpanNode{{SpanID: "child"}},
	}
	assert.Equal(t, 2, countSpanNode(node))
	assert.Equal(t, 2, countSpanNodes([]*types.SpanNode{node}))
	assert.Equal(t, 2, countSpanNodes([]types.SpanNode{{SpanID: "a"}, {SpanID: "b"}}))
	assert.Zero(t, countSpanNode((*types.SpanNode)(nil)))
	assert.Zero(t, countSpanNode("invalid"))
	assert.Zero(t, countSpanNodes("invalid"))
}

func TestSummarizeResponseEdgeCases(t *testing.T) {
	resp := summarizeResponse(types.GetServicesOutput{Services: []string{"a"}}, toolStatusError)
	assert.False(t, resp.hasResultCount)
	assert.Zero(t, resp.resultCount)

	resp = summarizeResponse("unsupported", toolStatusOK)
	assert.False(t, resp.hasResultCount)
	assert.Zero(t, resp.resultCount)
}

func TestAddOTelToolLabelsWithoutLabeler(t *testing.T) {
	assert.NotPanics(t, func() {
		addOTelToolLabels(context.Background(), "tool", toolStatusOK)
	})
}

func TestLogFailureErrorLevel(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	obs := newToolObservability(zap.New(core), metricstest.NewFactory(0))
	obs.logFailure(toolStatusError, zap.String("tool_name", "x"))
	entries := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, entries, 1)
	assert.Equal(t, zapcore.ErrorLevel, entries[0].Level)
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
