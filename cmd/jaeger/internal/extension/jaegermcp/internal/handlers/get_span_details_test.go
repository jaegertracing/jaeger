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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// Helper types and functions are defined in test_helpers.go

func TestGetSpanDetailsHandler_Handle_Success(t *testing.T) {
	traceID := testTraceID
	spanID1 := "span001"
	spanID2 := "span002"

	spanConfigs := []spanConfig{
		{
			spanID:    spanID1,
			operation: "/api/test1",
			attributes: map[string]string{
				"http.method": "GET",
				"http.url":    "/api/test1",
			},
		},
		{
			spanID:       spanID2,
			parentSpanID: spanID1,
			operation:    "/api/test2",
			hasError:     true,
			errorMessage: "Test error",
			attributes: map[string]string{
				"http.status_code": "500",
			},
			events: []eventConfig{
				{
					name: "error_event",
					attributes: map[string]string{
						"error.type": "TestError",
					},
				},
			},
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(spanID1), spanIDToHex(spanID2)},
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Len(t, output.Spans, 2)

	// Verify span details
	var span1, span2 *types.SpanDetail
	for i := range output.Spans {
		switch output.Spans[i].Operation {
		case "/api/test1":
			span1 = &output.Spans[i]
		case "/api/test2":
			span2 = &output.Spans[i]
		default:
			// Other operations not relevant to this test
		}
	}

	require.NotNil(t, span1)
	assert.Equal(t, "/api/test1", span1.Operation)
	assert.Equal(t, "Ok", span1.Status.Code)
	assert.Equal(t, "GET", span1.Attributes["http.method"])
	assert.Equal(t, "/api/test1", span1.Attributes["http.url"])

	require.NotNil(t, span2)
	assert.Equal(t, "/api/test2", span2.Operation)
	assert.Equal(t, "Error", span2.Status.Code)
	assert.Equal(t, "Test error", span2.Status.Message)
	assert.Equal(t, "500", span2.Attributes["http.status_code"])
	assert.Len(t, span2.Events, 1)
	assert.Equal(t, "error_event", span2.Events[0].Name)
}

func TestGetSpanDetailsHandler_Handle_SingleSpan(t *testing.T) {
	traceID := testTraceID
	spanID := "span001"

	spanConfigs := []spanConfig{
		{
			spanID:    spanID,
			operation: "/api/test",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(spanID)},
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Len(t, output.Spans, 1)
	assert.Equal(t, "/api/test", output.Spans[0].Operation)
}

func TestGetSpanDetailsHandler_Handle_FiltersBySpanIDs(t *testing.T) {
	traceID := testTraceID

	spanConfigs := []spanConfig{
		{spanID: "span001", operation: "/api/test1"},
		{spanID: "span002", operation: "/api/test2"},
		{spanID: "span003", operation: "/api/test3"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex("span001"), spanIDToHex("span003")}, // Only request span001 and span003
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 2)

	operations := make(map[string]bool)
	for _, span := range output.Spans {
		operations[span.Operation] = true
	}

	assert.True(t, operations["/api/test1"])
	assert.False(t, operations["/api/test2"]) // span002 should not be included
	assert.True(t, operations["/api/test3"])
}

func TestGetSpanDetailsHandler_Handle_MissingTraceID(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	input := types.GetSpanDetailsInput{
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetSpanDetailsHandler_Handle_MissingSpanIDs(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	input := types.GetSpanDetailsInput{
		TraceID: testTraceID,
		SpanIDs: []string{},
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "span_ids is required")
}

func TestGetSpanDetailsHandler_Handle_InvalidTraceID(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	input := types.GetSpanDetailsInput{
		TraceID: "invalid-trace-id",
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetSpanDetailsHandler_Handle_TraceNotFound(t *testing.T) {
	mock := newMockYieldingEmpty()

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: testTraceID,
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetSpanDetailsHandler_Handle_QueryError(t *testing.T) {
	mock := newMockYieldingError(errors.New("database connection failed"))

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: testTraceID,
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should return an error directly
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace")
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestGetSpanDetailsHandler_Handle_PartialResults(t *testing.T) {
	traceID := testTraceID
	spanID1 := "span001"

	// Create trace with one span
	testTrace1 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: spanID1, operation: "/api/test1"},
	})

	mock := &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				// Yield first batch successfully
				if !yield([]ptrace.Traces{testTrace1}, nil) {
					return // Stop if consumer doesn't want more
				}
				// Yield error - for singular lookups this should abort
				yield(nil, errors.New("database connection failed"))
			}
		},
	}

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(spanID1)},
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should return an error directly
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace")
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestGetSpanDetailsHandler_Handle_MultipleIterations(t *testing.T) {
	traceID := testTraceID
	spanID1 := "span001"
	spanID2 := "span002"

	// Create traces with different spans that will be merged
	testTrace1 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: spanID1, operation: "/api/test1"},
	})
	testTrace2 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: spanID2, operation: "/api/test2"},
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

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(spanID1), spanIDToHex(spanID2)},
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should succeed and return both spans
	require.NoError(t, err)
	assert.Len(t, output.Spans, 2)
}

func TestGetSpanDetailsHandler_Handle_WithParentSpanID(t *testing.T) {
	traceID := testTraceID
	parentSpanID := "parent001"
	childSpanID := "child001"

	spanConfigs := []spanConfig{
		{
			spanID:    parentSpanID,
			operation: "/parent",
		},
		{
			spanID:       childSpanID,
			parentSpanID: parentSpanID,
			operation:    "/child",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(childSpanID)},
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 1)
	assert.NotEmpty(t, output.Spans[0].ParentSpanID)
}

func TestGetSpanDetailsHandler_Handle_NoMatchingSpans(t *testing.T) {
	traceID := testTraceID

	spanConfigs := []spanConfig{
		{spanID: "span001", operation: "/api/test"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex("nonexistent_span")}, // Request a span that doesn't exist
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Empty(t, output.Spans) // No spans should be returned
}

func TestConvertAttributeValue(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() pcommon.Value
		expected any
	}{
		{
			name: "string value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueStr("test")
				return v
			},
			expected: "test",
		},
		{
			name: "int value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueInt(42)
				return v
			},
			expected: int64(42),
		},
		{
			name: "double value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueDouble(3.14)
				return v
			},
			expected: 3.14,
		},
		{
			name: "bool value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueBool(true)
				return v
			},
			expected: true,
		},
		{
			name: "slice value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueSlice()
				slice := v.Slice()
				slice.AppendEmpty().SetStr("item1")
				slice.AppendEmpty().SetStr("item2")
				return v
			},
			expected: []any{"item1", "item2"},
		},
		{
			name: "map value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueMap()
				m := v.Map()
				m.PutStr("key1", "value1")
				m.PutInt("key2", 123)
				return v
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": int64(123),
			},
		},
		{
			name: "bytes value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueBytes()
				v.Bytes().FromRaw([]byte{1, 2, 3})
				return v
			},
			expected: []byte{1, 2, 3},
		},
		{
			name:     "empty value",
			setup:    pcommon.NewValueEmpty,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.setup()
			result := convertAttributeValue(value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseTraceID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid trace ID",
			input:     "00000000000000000000000000000001",
			wantError: false,
		},
		{
			name:      "invalid trace ID - wrong length",
			input:     "invalid",
			wantError: true,
		},
		{
			name:      "invalid trace ID - non-hex characters",
			input:     "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			wantError: true,
		},
		{
			name:      "empty trace ID",
			input:     "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTraceID(tt.input)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, result.IsEmpty())
			}
		})
	}
}
