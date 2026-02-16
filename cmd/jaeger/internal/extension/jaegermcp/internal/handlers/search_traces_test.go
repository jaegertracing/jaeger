// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// mockQueryService and createTestTrace are defined in test_helpers.go

func TestSearchTracesHandler_Handle_Success(t *testing.T) {
	// Create test data
	testTrace := createTestTrace("trace123", "frontend", "/api/checkout", false)

	// Test buildTraceSummary directly
	summary := buildTraceSummary(testTrace)

	assert.Equal(t, "frontend", summary.RootService)
	assert.Equal(t, "/api/checkout", summary.RootSpanName)
	assert.Equal(t, 1, summary.SpanCount)
	assert.Equal(t, 1, summary.ServiceCount)
	assert.False(t, summary.HasErrors)
}

func TestSearchTracesHandler_BuildSummary_WithErrors(t *testing.T) {
	testTrace := createTestTrace("trace456", "payment", "/process", true)

	summary := buildTraceSummary(testTrace)

	assert.Equal(t, "payment", summary.RootService)
	assert.Equal(t, "/process", summary.RootSpanName)
	assert.True(t, summary.HasErrors)
}

func TestSearchTracesHandler_Handle_FullWorkflow(t *testing.T) {
	testTrace := createTestTrace("trace789", "cart-service", "/get-cart", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			// Verify query parameters
			assert.Equal(t, "cart-service", query.ServiceName)
			assert.Equal(t, "/get-cart", query.OperationName)
			assert.Equal(t, 10, query.SearchDepth) // Default
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "cart-service",
		SpanName:     "/get-cart",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
	assert.Equal(t, "cart-service", output.Traces[0].RootService)
	assert.Equal(t, "/get-cart", output.Traces[0].RootSpanName)
}

func TestSearchTracesHandler_Handle_WithStartTimeMax(t *testing.T) {
	testTrace := createTestTrace("trace999", "test-service", "/test", false)

	mock := newMockFindTraces(testTrace)

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-2h",
		StartTimeMax: "-1h",
		ServiceName:  "test-service",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
}

func TestSearchTracesHandler_Handle_WithDurations(t *testing.T) {
	testTrace := createTestTrace("trace888", "slow-service", "/slow", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			assert.Equal(t, 2*time.Second, query.DurationMin)
			assert.Equal(t, 10*time.Second, query.DurationMax)
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "slow-service",
		DurationMin:  "2s",
		DurationMax:  "10s",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
}

func TestSearchTracesHandler_Handle_WithAttributes(t *testing.T) {
	testTrace := createTestTrace("trace777", "api-service", "/api", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			// Verify attributes were converted
			statusCode, ok := query.Attributes.Get("http.status_code")
			assert.True(t, ok)
			assert.Equal(t, "500", statusCode.Str())
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "api-service",
		Attributes: map[string]string{
			"http.status_code": "500",
		},
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
}

func TestSearchTracesHandler_Handle_WithErrorsFilter(t *testing.T) {
	errorTrace := createTestTrace("trace666", "error-service", "/error", true)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			// Verify that error attribute filter is added
			val, ok := query.Attributes.Get("error")
			assert.True(t, ok)
			assert.Equal(t, "true", val.Str())

			return func(yield func([]ptrace.Traces, error) bool) {
				// Return only error traces (simulating storage filtering)
				yield([]ptrace.Traces{errorTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "error-service",
		WithErrors:   true,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	// Only error trace should be returned
	require.Len(t, output.Traces, 1)
	assert.True(t, output.Traces[0].HasErrors)
}

func TestSearchTracesHandler_Handle_SearchDepthDefault(t *testing.T) {
	testTrace := createTestTrace("trace444", "test", "/test", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			assert.Equal(t, 10, query.SearchDepth) // Default value
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		SearchDepth:  0, // Should use default
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)
}

func TestSearchTracesHandler_Handle_SearchDepthMax(t *testing.T) {
	testTrace := createTestTrace("trace333", "test", "/test", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			assert.Equal(t, 100, query.SearchDepth) // Max value
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		SearchDepth:  200, // Should be capped at 100
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)
}

func TestSearchTracesHandler_Handle_QueryError(t *testing.T) {
	// Create a test trace to return before the error
	testTrace := createTestTrace("trace123", "test-service", "/api/test", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				// Yield one trace first, then an error
				yield([]ptrace.Traces{testTrace}, nil)
				yield(nil, errors.New("database connection failed"))
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should not return an error - instead returns partial results with Error field
	require.NoError(t, err)
	assert.Len(t, output.Traces, 1)
	assert.NotEmpty(t, output.Error)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "database connection failed")
}

func TestSearchTracesHandler_Handle_PartialResults(t *testing.T) {
	// Create test traces
	testTrace1 := createTestTrace("trace001", "test-service", "/api/test1", false)
	testTrace2 := createTestTrace("trace002", "test-service", "/api/test2", false)
	testTrace3 := createTestTrace("trace003", "test-service", "/api/test3", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				// Yield first batch successfully
				yield([]ptrace.Traces{testTrace1}, nil)
				// Yield error in the middle
				yield(nil, errors.New("temporary failure"))
				// Yield remaining batches successfully
				yield([]ptrace.Traces{testTrace2}, nil)
				yield([]ptrace.Traces{testTrace3}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should not return an error - instead returns partial results with Error field
	require.NoError(t, err)
	// Should have all 3 traces since we continue processing after error
	assert.Len(t, output.Traces, 3)
	assert.NotEmpty(t, output.Error)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "temporary failure")
}

func TestSearchTracesHandler_Handle_MissingServiceName(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		// Missing ServiceName
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name is required")
}

func TestSearchTracesHandler_Handle_InvalidTimeFormat(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "invalid-time",
		ServiceName:  "test",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_time_min")
}

func TestSearchTracesHandler_Handle_InvalidStartTimeMax(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		StartTimeMax: "invalid-time",
		ServiceName:  "test",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_time_max")
}

func TestSearchTracesHandler_Handle_InvalidDurationMin(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMin:  "invalid-duration",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration_min")
}

func TestSearchTracesHandler_Handle_InvalidDurationMax(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMax:  "invalid-duration",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration_max")
}

func TestSearchTracesHandler_Handle_DurationMaxLessThanMin(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMin:  "10s",
		DurationMax:  "5s",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration_max must be greater than duration_min")
}

func TestParseTimeParam(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "now",
			input:     "now",
			wantError: false,
		},
		{
			name:      "relative hours",
			input:     "-1h",
			wantError: false,
		},
		{
			name:      "relative minutes",
			input:     "-30m",
			wantError: false,
		},
		{
			name:      "relative seconds",
			input:     "-5s",
			wantError: false,
		},
		{
			name:      "RFC3339",
			input:     "2024-01-15T10:30:00Z",
			wantError: false,
		},
		{
			name:      "invalid format",
			input:     "invalid",
			wantError: true,
		},
		{
			name:      "invalid relative",
			input:     "-invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTimeParam(tt.input)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, result.IsZero())
			}
		})
	}
}

func TestSearchTracesHandler_Handle_DefaultStartTime(t *testing.T) {
	testTrace := createTestTrace("traceDefault", "default-service", "/test", false)

	mock := &mockQueryService{
		findTracesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			// Verify that start time was set (not zero), implying default was applied or computed
			assert.False(t, query.StartTimeMin.IsZero())
			// We can't easily verify exact -1h without mocking time.Now, but we know it should be approximately 1h ago
			assert.WithinDuration(t, time.Now().Add(-1*time.Hour), query.StartTimeMin, 5*time.Second)
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock}

	// Omit StartTimeMin to trigger default logic
	input := types.SearchTracesInput{
		ServiceName: "default-service",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
}
