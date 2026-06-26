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
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
)

var testTraceIDBytes = func() pcommon.TraceID {
	tid := pcommon.TraceID{}
	copy(tid[:], testTraceID)
	return tid
}()

func makeTraceSummary(rootService, rootOp string, hasError bool) tracestore.TraceSummary {
	errorCount := 0
	if hasError {
		errorCount = 1
	}
	return tracestore.TraceSummary{
		TraceID:           testTraceIDBytes,
		RootServiceName:   rootService,
		RootOperationName: rootOp,
		MinStartTime:      time.Now().Add(-5 * time.Second),
		MaxEndTime:        time.Now(),
		SpanCount:         1,
		ErrorSpanCount:    errorCount,
		Services: []tracestore.ServiceSummary{
			{Name: rootService, SpanCount: 1, ErrorSpanCount: errorCount},
		},
	}
}

func TestToMCPTraceSummary_Basic(t *testing.T) {
	s := makeTraceSummary("frontend", "/api/checkout", false)
	out := toMCPTraceSummary(s)

	assert.Equal(t, "frontend", out.RootService)
	assert.Equal(t, "/api/checkout", out.RootSpanName)
	assert.Equal(t, 1, out.SpanCount)
	assert.Equal(t, 1, out.ServiceCount)
	assert.Equal(t, []string{"frontend"}, out.Services)
	assert.False(t, out.HasErrors)
	assert.NotEmpty(t, out.StartTime)
	assert.Positive(t, out.DurationUs)
}

func TestToMCPTraceSummary_WithErrors(t *testing.T) {
	s := makeTraceSummary("payment", "/process", true)
	out := toMCPTraceSummary(s)

	assert.Equal(t, "payment", out.RootService)
	assert.Equal(t, "/process", out.RootSpanName)
	assert.True(t, out.HasErrors)
}

func TestToMCPTraceSummary_MultipleServices(t *testing.T) {
	s := tracestore.TraceSummary{
		TraceID:           testTraceIDBytes,
		RootServiceName:   "api-gateway",
		RootOperationName: "/op",
		SpanCount:         3,
		Services: []tracestore.ServiceSummary{
			{Name: "api-gateway", SpanCount: 1},
			{Name: "payment", SpanCount: 1},
			{Name: "user-service", SpanCount: 1},
		},
	}
	out := toMCPTraceSummary(s)

	assert.Equal(t, 3, out.ServiceCount)
	// Services list is already sorted in tracestore.TraceSummary
	assert.Equal(t, []string{"api-gateway", "payment", "user-service"}, out.Services)
}

func TestSearchTracesHandler_Handle_FullWorkflow(t *testing.T) {
	want := makeTraceSummary("cart-service", "/get-cart", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			assert.Equal(t, "cart-service", query.ServiceName)
			assert.Equal(t, "/get-cart", query.OperationName)
			assert.Equal(t, 10, query.SearchDepth)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

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
	want := makeTraceSummary("test-service", "/test", false)
	mock := newMockFindTraceSummaries(want)

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

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
	want := makeTraceSummary("slow-service", "/slow", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			assert.Equal(t, 2*time.Second, query.DurationMin)
			assert.Equal(t, 10*time.Second, query.DurationMax)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

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
	want := makeTraceSummary("api-service", "/api", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			statusCode, ok := query.Attributes.Get("http.status_code")
			assert.True(t, ok)
			assert.Equal(t, "500", statusCode.Str())
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

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
	errSummary := makeTraceSummary("error-service", "/error", true)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			errorAttr, ok := query.Attributes.Get("error")
			assert.True(t, ok)
			assert.Equal(t, "true", errorAttr.Str())
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{errSummary}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "error-service",
		WithErrors:   true,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
	assert.True(t, output.Traces[0].HasErrors)
}

func TestSearchTracesHandler_Handle_WithErrorsFilter_UsingMemoryStore(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)

	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace(
		"trace111",
		"test",
		"/ok",
		false,
	)))
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace(
		"trace222",
		"test",
		"/error",
		true,
	)))

	handler := &searchTracesHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		WithErrors:   true,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
	assert.True(t, output.Traces[0].HasErrors)
	assert.Equal(t, "test", output.Traces[0].RootService)
	assert.Equal(t, "/error", output.Traces[0].RootSpanName)
	assert.Empty(t, output.Error)
}

func TestSearchTracesHandler_Handle_SearchDepthDefault(t *testing.T) {
	want := makeTraceSummary("test", "/test", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			assert.Equal(t, 10, query.SearchDepth)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		SearchDepth:  0,
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)
}

func TestSearchTracesHandler_Handle_SearchDepthMax(t *testing.T) {
	want := makeTraceSummary("test", "/test", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			assert.Equal(t, 100, query.SearchDepth)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		SearchDepth:  200, // capped at maxResults=100
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)
}

func TestSearchTracesHandler_Handle_QueryError(t *testing.T) {
	want := makeTraceSummary("test-service", "/api/test", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
				yield(nil, errors.New("database connection failed"))
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Traces, 1)
	assert.NotEmpty(t, output.Error)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "database connection failed")
}

func TestSearchTracesHandler_Handle_PartialResults(t *testing.T) {
	s1 := makeTraceSummary("test-service", "/api/test1", false)
	s2 := makeTraceSummary("test-service", "/api/test2", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				if !yield([]tracestore.TraceSummary{s1}, nil) {
					return
				}
				// Caller returns false after seeing the error, so s2 is not delivered.
				if !yield(nil, errors.New("temporary failure")) {
					return
				}
				yield([]tracestore.TraceSummary{s2}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Traces, 1)
	assert.NotEmpty(t, output.Error)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "temporary failure")
}

func TestSearchTracesHandler_Handle_MissingServiceName(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name is required")
}

func TestSearchTracesHandler_Handle_InvalidTimeFormat(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

	input := types.SearchTracesInput{
		StartTimeMin: "invalid-time",
		ServiceName:  "test",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_time_min")
}

func TestSearchTracesHandler_Handle_InvalidStartTimeMax(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

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
	handler := NewSearchTracesHandler(nil, 100)

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
	handler := NewSearchTracesHandler(nil, 100)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMax:  "invalid-duration",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration_max")
}

func TestSearchTracesHandler_Handle_StartTimeMaxBeforeMin(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		StartTimeMax: "-2h",
		ServiceName:  "test",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "start_time_max must be after start_time_min")
}

func TestSearchTracesHandler_Handle_DurationMaxLessThanMin(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

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
	want := makeTraceSummary("default-service", "/test", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			assert.False(t, query.StartTimeMin.IsZero())
			assert.WithinDuration(t, time.Now().Add(-1*time.Hour), query.StartTimeMin, 5*time.Second)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		ServiceName: "default-service",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
}

func TestSearchTracesHandler_Handle_LimitEnforced(t *testing.T) {
	summaries := []tracestore.TraceSummary{
		makeTraceSummary("svc", "/op1", false),
		makeTraceSummary("svc", "/op2", false),
		makeTraceSummary("svc", "/op3", false),
		makeTraceSummary("svc", "/op4", false),
		makeTraceSummary("svc", "/op5", false),
	}

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				for _, s := range summaries {
					if !yield([]tracestore.TraceSummary{s}, nil) {
						return
					}
				}
			}
		},
	}

	handler := &searchTracesHandler{queryService: mock, maxResults: 3}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "svc",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Traces, 3)
}

func TestSearchTracesHandler_Handle_NegativeDurationMin(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMin:  "-5s",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration_min cannot be negative")
}

func TestSearchTracesHandler_Handle_NegativeDurationMax(t *testing.T) {
	handler := NewSearchTracesHandler(nil, 100)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMax:  "-10s",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration_max cannot be negative")
}
