// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	input := types.GetSpanNamesInput{
		ServiceName: "checkout",
		Limit:       25,
	}
	result, output, err := wrapped(context.Background(), nil, input)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Len(t, output.SpanNames, 2)

	payload, err := json.Marshal(output)
	require.NoError(t, err)

	metricsFactory.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{
			Name:  "requests",
			Tags:  map[string]string{"tool_name": "get_span_names", "status": "ok"},
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "response_items",
			Tags:  map[string]string{"tool_name": "get_span_names", "status": "ok"},
			Value: 2,
		},
		metricstest.ExpectedMetric{
			Name:  "response_bytes",
			Tags:  map[string]string{"tool_name": "get_span_names", "status": "ok"},
			Value: len(payload),
		},
	)
	metricsFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "in_flight_requests",
		Tags:  map[string]string{"tool_name": "get_span_names"},
		Value: 0,
	})

	_, gauges := metricsFactory.Snapshot()
	_, hasLatency := gauges["latency|status=ok|tool_name=get_span_names.P50"]
	assert.True(t, hasLatency)

	startLogs := observed.FilterMessage("MCP tool invocation started").All()
	require.Len(t, startLogs, 1)
	startContext := startLogs[0].ContextMap()
	assert.Equal(t, "get_span_names", startContext["tool_name"])
	assert.Equal(t, "checkout", startContext["service_name"])
	assert.EqualValues(t, 25, startContext["requested_limit"])

	doneLogs := observed.FilterMessage("MCP tool invocation completed").All()
	require.Len(t, doneLogs, 1)
	doneContext := doneLogs[0].ContextMap()
	assert.Equal(t, "get_span_names", doneContext["tool_name"])
	assert.Equal(t, "ok", doneContext["status"])
	assert.EqualValues(t, 2, doneContext["result_count"])
	assert.EqualValues(t, len(payload), doneContext["response_size_bytes"])
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

	_, _, err := wrapped(context.Background(), nil, types.GetTraceTopologyInput{TraceID: "deadbeef"})
	require.ErrorIs(t, err, expectedErr)

	metricsFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "requests",
		Tags:  map[string]string{"tool_name": "get_trace_topology", "status": "not_found"},
		Value: 1,
	})
	metricsFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "in_flight_requests",
		Tags:  map[string]string{"tool_name": "get_trace_topology"},
		Value: 0,
	})

	failedLogs := observed.FilterMessage("MCP tool invocation failed").All()
	require.Len(t, failedLogs, 1)
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
		{
			name: "ok",
			want: toolStatusOK,
		},
		{
			name: "invalid argument",
			err:  errors.New("service_name is required"),
			want: toolStatusInvalidArgument,
		},
		{
			name: "not found",
			err:  errors.New("trace not found"),
			want: toolStatusNotFound,
		},
		{
			name: "generic error",
			err:  errors.New("storage backend unavailable"),
			want: toolStatusError,
		},
		{
			name:   "result error not found",
			result: resultWithNotFound,
			want:   toolStatusNotFound,
		},
		{
			name:   "result error generic",
			result: &mcp.CallToolResult{IsError: true},
			want:   toolStatusError,
		},
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
			{
				SpanID: "child-b",
				Children: []*types.SpanNode{
					{SpanID: "grandchild"},
				},
			},
		},
	}
	orphans := []*types.SpanNode{
		{SpanID: "orphan"},
	}

	count, ok := inferResultCount(types.GetTraceTopologyOutput{
		RootSpan: root,
		Orphans:  orphans,
	})
	require.True(t, ok)
	assert.Equal(t, 5, count)
}
