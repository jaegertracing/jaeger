// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

func TestGetTraceErrorsHandler_Handle_Success(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{
			spanID:    "span001",
			operation: "/api/ok",
			hasError:  false,
		},
		{
			spanID:       "span002",
			operation:    "/api/error1",
			hasError:     true,
			errorMessage: "First error",
			attributes: map[string]string{
				"error.type": "NetworkError",
			},
		},
		{
			spanID:       "span003",
			operation:    "/api/error2",
			hasError:     true,
			errorMessage: "Second error",
			attributes: map[string]string{
				"error.type": "TimeoutError",
			},
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 2, output.ErrorCount)
	assert.Len(t, output.Spans, 2)

	// Verify only error spans are returned
	for _, span := range output.Spans {
		assert.Equal(t, "Error", span.Status.Code)
		assert.NotEmpty(t, span.Status.Message)
	}

	// Verify both error operations are present
	operations := make(map[string]bool)
	for _, span := range output.Spans {
		operations[span.Operation] = true
	}
	assert.True(t, operations["/api/error1"])
	assert.True(t, operations["/api/error2"])
	assert.False(t, operations["/api/ok"]) // OK span should not be included
}

func TestGetTraceErrorsHandler_Handle_NoErrors(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{
			spanID:    "span001",
			operation: "/api/ok1",
			hasError:  false,
		},
		{
			spanID:    "span002",
			operation: "/api/ok2",
			hasError:  false,
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 0, output.ErrorCount)
	assert.Empty(t, output.Spans)
}

func TestGetTraceErrorsHandler_Handle_SingleError(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{
			spanID:    "span001",
			operation: "/api/ok",
			hasError:  false,
		},
		{
			spanID:       "span002",
			operation:    "/api/error",
			hasError:     true,
			errorMessage: "Single error",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 1, output.ErrorCount)
	assert.Len(t, output.Spans, 1)
	assert.Equal(t, "/api/error", output.Spans[0].Operation)
	assert.Equal(t, "Error", output.Spans[0].Status.Code)
	assert.Equal(t, "Single error", output.Spans[0].Status.Message)
}

func TestGetTraceErrorsHandler_Handle_MissingTraceID(t *testing.T) {
	handler := NewGetTraceErrorsHandler(nil)

	input := types.GetTraceErrorsInput{
		TraceID: "",
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetTraceErrorsHandler_Handle_InvalidTraceID(t *testing.T) {
	handler := NewGetTraceErrorsHandler(nil)

	input := types.GetTraceErrorsInput{
		TraceID: "invalid-trace-id",
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetTraceErrorsHandler_Handle_TraceNotFound(t *testing.T) {
	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(_ func([]ptrace.Traces, error) bool) {
				// Don't yield any traces
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: "12345678901234567890123456789012",
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetTraceErrorsHandler_Handle_QueryError(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	// Create a trace with an error span, but return it before the error
	spanConfigs := []spanConfig{
		{
			spanID:       "span001",
			operation:    "/api/error",
			hasError:     true,
			errorMessage: "Test error",
		},
	}
	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				// Yield the trace first, then an error
				yield([]ptrace.Traces{testTrace}, nil)
				yield(nil, errors.New("database connection failed"))
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should not return an error - instead returns partial results with Error field
	require.NoError(t, err)
	assert.Equal(t, 1, output.ErrorCount)
	assert.Len(t, output.Spans, 1)
	assert.NotEmpty(t, output.Error)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "database connection failed")
}

func TestGetTraceErrorsHandler_Handle_AllSpansHaveErrors(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{
			spanID:       "span001",
			operation:    "/api/error1",
			hasError:     true,
			errorMessage: "Error 1",
		},
		{
			spanID:       "span002",
			operation:    "/api/error2",
			hasError:     true,
			errorMessage: "Error 2",
		},
		{
			spanID:       "span003",
			operation:    "/api/error3",
			hasError:     true,
			errorMessage: "Error 3",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 3, output.ErrorCount)
	assert.Len(t, output.Spans, 3)

	// Verify all spans have error status
	for _, span := range output.Spans {
		assert.Equal(t, "Error", span.Status.Code)
		assert.NotEmpty(t, span.Status.Message)
	}
}

func TestGetTraceErrorsHandler_Handle_ErrorSpanAttributes(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{
			spanID:       "span001",
			operation:    "/api/error",
			hasError:     true,
			errorMessage: "Test error",
			attributes: map[string]string{
				"http.status_code": "500",
				"error.type":       "InternalServerError",
				"error.message":    "Database connection failed",
			},
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 1)

	span := output.Spans[0]
	assert.Equal(t, "500", span.Attributes["http.status_code"])
	assert.Equal(t, "InternalServerError", span.Attributes["error.type"])
	assert.Equal(t, "Database connection failed", span.Attributes["error.message"])
}

func TestGetTraceErrorsHandler_Handle_ErrorSpanWithEvents(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{
			spanID:       "span001",
			operation:    "/api/error",
			hasError:     true,
			errorMessage: "Test error",
			events: []eventConfig{
				{
					name: "exception",
					attributes: map[string]string{
						"exception.type":    "RuntimeError",
						"exception.message": "Something went wrong",
					},
				},
			},
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 1)

	span := output.Spans[0]
	assert.Len(t, span.Events, 1)
	assert.Equal(t, "exception", span.Events[0].Name)
	assert.Equal(t, "RuntimeError", span.Events[0].Attributes["exception.type"])
	assert.Equal(t, "Something went wrong", span.Events[0].Attributes["exception.message"])
}
