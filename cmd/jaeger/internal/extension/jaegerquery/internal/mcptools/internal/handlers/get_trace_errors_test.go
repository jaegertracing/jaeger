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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

func TestGetTraceErrorsHandler_Handle_Success(t *testing.T) {
	traceID := testTraceID

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

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 2, output.TotalErrorCount)
	assert.Len(t, output.Spans, 2)

	// Verify only error spans are returned
	for _, span := range output.Spans {
		assert.Equal(t, "Error", span.Status.Code)
		assert.NotEmpty(t, span.Status.Message)
	}

	// Verify both error operations are present
	operations := make(map[string]bool)
	for _, span := range output.Spans {
		operations[span.SpanName] = true
	}
	assert.True(t, operations["/api/error1"])
	assert.True(t, operations["/api/error2"])
	assert.False(t, operations["/api/ok"]) // OK span should not be included
}

func TestGetTraceErrorsHandler_Handle_NoErrors(t *testing.T) {
	traceID := testTraceID

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

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 0, output.TotalErrorCount)
	assert.Empty(t, output.Spans)
}

func TestGetTraceErrorsHandler_Handle_SingleError(t *testing.T) {
	traceID := testTraceID

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

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 1, output.TotalErrorCount)
	assert.Len(t, output.Spans, 1)
	assert.Equal(t, "/api/error", output.Spans[0].SpanName)
	assert.Equal(t, "Error", output.Spans[0].Status.Code)
	assert.Equal(t, "Single error", output.Spans[0].Status.Message)
}

func TestGetTraceErrorsHandler_Handle_MissingTraceID(t *testing.T) {
	handler := NewGetTraceErrorsHandler(nil, 100)

	input := types.GetTraceErrorsInput{
		TraceID: "",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetTraceErrorsHandler_Handle_InvalidTraceID(t *testing.T) {
	handler := NewGetTraceErrorsHandler(nil, 100)

	input := types.GetTraceErrorsInput{
		TraceID: "invalid-trace-id",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetTraceErrorsHandler_Handle_TraceNotFound(t *testing.T) {
	mock := newMockYieldingEmpty()

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: testTraceID,
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetTraceErrorsHandler_Handle_QueryError(t *testing.T) {
	mock := newMockYieldingError(errors.New("database connection failed"))

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: testTraceID,
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should return the error directly
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestGetTraceErrorsHandler_Handle_MultipleIterations(t *testing.T) {
	traceID := testTraceID

	// Create traces with error spans that will be merged
	testTrace1 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: "span001", operation: "/api/error1", hasError: true, errorMessage: "Error 1"},
	})
	testTrace2 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: "span002", operation: "/api/error2", hasError: true, errorMessage: "Error 2"},
	})

	mock := &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				// Yield multiple batches successfully - they should be merged
				yield([]ptrace.Traces{testTrace1}, nil)
				yield([]ptrace.Traces{testTrace2}, nil)
			}
		},
	}

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should succeed and return both error spans
	require.NoError(t, err)
	assert.Equal(t, 2, output.TotalErrorCount)
	assert.Len(t, output.Spans, 2)
}

func TestGetTraceErrorsHandler_Handle_AllSpansHaveErrors(t *testing.T) {
	traceID := testTraceID

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

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 3, output.TotalErrorCount)
	assert.Len(t, output.Spans, 3)

	// Verify all spans have error status
	for _, span := range output.Spans {
		assert.Equal(t, "Error", span.Status.Code)
		assert.NotEmpty(t, span.Status.Message)
	}
}

func TestGetTraceErrorsHandler_Handle_ErrorSpanAttributes(t *testing.T) {
	traceID := testTraceID

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

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 1)

	span := output.Spans[0]
	assert.Equal(t, "500", span.Attributes["http.status_code"])
	assert.Equal(t, "InternalServerError", span.Attributes["error.type"])
	assert.Equal(t, "Database connection failed", span.Attributes["error.message"])
}

func TestGetTraceErrorsHandler_Handle_ErrorSpanWithEvents(t *testing.T) {
	traceID := testTraceID

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

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{queryService: mock}

	input := types.GetTraceErrorsInput{
		TraceID: traceID,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 1)

	span := output.Spans[0]
	assert.Len(t, span.Events, 1)
	assert.Equal(t, "exception", span.Events[0].Name)
	assert.Equal(t, "RuntimeError", span.Events[0].Attributes["exception.type"])
	assert.Equal(t, "Something went wrong", span.Events[0].Attributes["exception.message"])
}

// TestGetTraceErrorsHandler_Handle_LimitEnforced verifies that when the number of error
// spans exceeds the configured limit, TotalErrorCount still reflects the full count and
// exactly `limit` spans are returned.
func TestGetTraceErrorsHandler_Handle_LimitEnforced(t *testing.T) {
	traceID := testTraceID

	// Create 5 error spans
	spanConfigs := []spanConfig{
		{spanID: "span001", operation: "/api/error1", hasError: true, errorMessage: "err1"},
		{spanID: "span002", operation: "/api/error2", hasError: true, errorMessage: "err2"},
		{spanID: "span003", operation: "/api/error3", hasError: true, errorMessage: "err3"},
		{spanID: "span004", operation: "/api/error4", hasError: true, errorMessage: "err4"},
		{spanID: "span005", operation: "/api/error5", hasError: true, errorMessage: "err5"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)

	// Set limit to 3 — should return at most 3 spans
	handler := &getTraceErrorsHandler{
		queryService:             mock,
		maxSpanDetailsPerRequest: 3,
	}

	input := types.GetTraceErrorsInput{TraceID: traceID}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	// TotalErrorCount reflects all error spans in the full trace (unbounded aggregation).
	assert.Equal(t, 5, output.TotalErrorCount)
	// Returned Spans are capped at exactly the limit (5 errors, limit=3 → exactly 3 spans).
	assert.Len(t, output.Spans, 3)
}

// TestGetTraceErrorsHandler_Handle_RootCausePrioritized is the regression test for the
// exact scenario described in the issue: a propagated failure chain where the originating
// error is on the deepest span. It verifies that the deepest error span is always present
// in the truncated response regardless of the order spans arrive from storage.
func TestGetTraceErrorsHandler_Handle_RootCausePrioritized(t *testing.T) {
	traceID := testTraceID

	// Chain: frontend (root, depth 0) → orders (depth 1) → payments (depth 2, originating error)
	// All three spans are in error. With limit=2, payments must always be included.
	//
	// spanID strings are 8 bytes max (pcommon.SpanID is [8]byte).
	// createTestTraceWithSpans copies the string bytes directly into the array.
	frontendID := "frontend"
	ordersID := "orders00"
	paymentsID := "payments"

	forwardOrder := []spanConfig{
		{spanID: frontendID, operation: "/checkout", hasError: true, errorMessage: "downstream error"},
		{spanID: ordersID, parentSpanID: frontendID, operation: "/place-order", hasError: true, errorMessage: "downstream error"},
		{spanID: paymentsID, parentSpanID: ordersID, operation: "/charge", hasError: true, errorMessage: "payment declined"},
	}
	reversedOrder := []spanConfig{
		{spanID: paymentsID, parentSpanID: ordersID, operation: "/charge", hasError: true, errorMessage: "payment declined"},
		{spanID: ordersID, parentSpanID: frontendID, operation: "/place-order", hasError: true, errorMessage: "downstream error"},
		{spanID: frontendID, operation: "/checkout", hasError: true, errorMessage: "downstream error"},
	}

	paymentsHex := spanIDToHex(paymentsID)
	ordersHex := spanIDToHex(ordersID)

	for _, tc := range []struct {
		name    string
		configs []spanConfig
	}{
		{"forward storage order", forwardOrder},
		{"reversed storage order", reversedOrder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testTrace := createTestTraceWithSpans(traceID, tc.configs)
			mock := newMockYieldingTraces(testTrace)

			handler := &getTraceErrorsHandler{
				queryService:             mock,
				maxSpanDetailsPerRequest: 2,
			}

			input := types.GetTraceErrorsInput{TraceID: traceID}
			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

			require.NoError(t, err)
			assert.Equal(t, 3, output.TotalErrorCount)
			require.Len(t, output.Spans, 2)

			// The deepest error (payments, depth 2) must always be first.
			assert.Equal(t, paymentsHex, output.Spans[0].SpanID, "payments span should be first (deepest)")
			// The next deepest (orders, depth 1) must always be second.
			assert.Equal(t, ordersHex, output.Spans[1].SpanID, "orders span should be second")
			// The root (frontend, depth 0) must be dropped.
			for _, s := range output.Spans {
				assert.NotEqual(t, spanIDToHex(frontendID), s.SpanID, "frontend root span should be dropped")
			}
		})
	}
}

// TestGetTraceErrorsHandler_Handle_DeterministicTieBreaking verifies that when two error
// spans are at the same depth (siblings), the same span is always selected regardless of
// which one arrives first from storage. Tie-breaking is by span ID ascending.
func TestGetTraceErrorsHandler_Handle_DeterministicTieBreaking(t *testing.T) {
	traceID := testTraceID

	// Two sibling error spans (both children of the same root); limit=1.
	// spanID "aaa" < "bbb" lexicographically, so "aaa" should always be kept.
	rootID := "root0000"
	aaaID := "aaaa0000"
	bbbID := "bbbb0000"

	forwardOrder := []spanConfig{
		{spanID: rootID, operation: "/root", hasError: false},
		{spanID: aaaID, parentSpanID: rootID, operation: "/aaa", hasError: true, errorMessage: "err"},
		{spanID: bbbID, parentSpanID: rootID, operation: "/bbb", hasError: true, errorMessage: "err"},
	}
	reversedOrder := []spanConfig{
		{spanID: rootID, operation: "/root", hasError: false},
		{spanID: bbbID, parentSpanID: rootID, operation: "/bbb", hasError: true, errorMessage: "err"},
		{spanID: aaaID, parentSpanID: rootID, operation: "/aaa", hasError: true, errorMessage: "err"},
	}

	aaaHex := spanIDToHex(aaaID)

	for _, tc := range []struct {
		name    string
		configs []spanConfig
	}{
		{"aaa before bbb", forwardOrder},
		{"bbb before aaa", reversedOrder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testTrace := createTestTraceWithSpans(traceID, tc.configs)
			mock := newMockYieldingTraces(testTrace)

			handler := &getTraceErrorsHandler{
				queryService:             mock,
				maxSpanDetailsPerRequest: 1,
			}

			input := types.GetTraceErrorsInput{TraceID: traceID}
			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

			require.NoError(t, err)
			assert.Equal(t, 2, output.TotalErrorCount)
			require.Len(t, output.Spans, 1)
			assert.Equal(t, aaaHex, output.Spans[0].SpanID,
				"span with lexicographically smaller ID should always win tie-break")
		})
	}
}

// TestGetTraceErrorsHandler_Handle_MixedDepthErrors verifies that non-error spans in the
// parent chain are still used to compute depth correctly. The deepest error span should
// always rank first even when its ancestors are not in error.
func TestGetTraceErrorsHandler_Handle_MixedDepthErrors(t *testing.T) {
	traceID := testTraceID

	// root (ok) → middle (ok) → deep (error)   depth=2
	//           → shallow (error)              depth=1
	// Limit=1. deep must win.
	rootID := "root0000"
	middleID := "middle00"
	deepID := "deep0000"
	shallowID := "shallow0"

	configs := []spanConfig{
		{spanID: rootID, operation: "/root", hasError: false},
		{spanID: middleID, parentSpanID: rootID, operation: "/middle", hasError: false},
		{spanID: deepID, parentSpanID: middleID, operation: "/deep", hasError: true, errorMessage: "deep err"},
		{spanID: shallowID, parentSpanID: rootID, operation: "/shallow", hasError: true, errorMessage: "shallow err"},
	}

	testTrace := createTestTraceWithSpans(traceID, configs)
	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceErrorsHandler{
		queryService:             mock,
		maxSpanDetailsPerRequest: 1,
	}

	input := types.GetTraceErrorsInput{TraceID: traceID}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, 2, output.TotalErrorCount)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, spanIDToHex(deepID), output.Spans[0].SpanID,
		"deeper error span (depth 2) should be selected over shallower one (depth 1)")
}
