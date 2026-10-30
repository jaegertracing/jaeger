// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
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
			assert.EqualValues(t, 10, query.SearchDepth)
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
			assert.EqualValues(t, 10, query.SearchDepth)
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
			assert.EqualValues(t, 100, query.SearchDepth)
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

func TestSearchTracesHandler_Handle_SearchDepthUnlimitedMaxResults(t *testing.T) {
	want := makeTraceSummary("test", "/test", false)

	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			assert.EqualValues(t, 25, query.SearchDepth)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}

	// maxResults == 0 means "no global cap", so an explicit search_depth must be preserved.
	handler := &searchTracesHandler{queryService: mock, maxResults: 0}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		SearchDepth:  25,
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

// TestSearchTracesHandler_Handle_NoServiceName covers the cross-service search an agent can
// now ask for: the tool forwards a query with no service name instead of refusing it, so an
// agent asked for "HTTP 500s in the last 10 minutes" makes one call rather than fanning out
// over get_services (RFC 0013).
func TestSearchTracesHandler_Handle_NoServiceName(t *testing.T) {
	var gotQuery querysvc.TraceQueryParams
	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			gotQuery = query
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{{RootServiceName: "svc-a"}}, nil)
			}
		},
	}
	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.SearchTracesInput{
		StartTimeMin: "-1h",
		Attributes:   map[string]string{"http.status_code": "500"},
	})

	require.NoError(t, err)
	assert.Empty(t, gotQuery.ServiceName, "the empty service name must reach the query service")
	assert.Len(t, output.Traces, 1)
	assert.Empty(t, output.Error)
}

// TestSearchTracesHandler_Handle_ServiceNameRequiredByBackend covers the other side: the
// query service refuses the query for a backend that cannot serve it, and the agent is told
// why rather than being handed an empty result set described as partial.
func TestSearchTracesHandler_Handle_ServiceNameRequiredByBackend(t *testing.T) {
	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield(nil, querysvc.ErrServiceNameRequired)
			}
		},
	}
	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.SearchTracesInput{
		StartTimeMin: "-1h",
	})

	require.NoError(t, err)
	assert.Empty(t, output.Traces)
	assert.Contains(t, output.Error, "requires a service name")
	assert.NotContains(t, output.Error, "partial results",
		"nothing was returned, so the agent must not be told results were partial")
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

// TestSearchTracesHandler_Handle_EnvelopeRefusedByQueryService covers the checks this tool no
// longer makes itself: an inverted time range and a negative or inverted duration bound are the
// query service's to refuse, and its refusal is what the agent reads. Nothing is returned, so
// the answer names the refusal rather than calling the results partial.
func TestSearchTracesHandler_Handle_EnvelopeRefusedByQueryService(t *testing.T) {
	tests := []struct {
		name    string
		input   types.SearchTracesInput
		wantErr string
	}{
		{
			name:    "start_time_max before start_time_min",
			input:   types.SearchTracesInput{StartTimeMin: "-1h", StartTimeMax: "-2h", ServiceName: "test"},
			wantErr: "min start time must be before max start time",
		},
		{
			name:    "duration_max less than duration_min",
			input:   types.SearchTracesInput{StartTimeMin: "-1h", ServiceName: "test", DurationMin: "10s", DurationMax: "5s"},
			wantErr: "max duration cannot be less than min duration",
		},
		{
			name:    "negative duration_min",
			input:   types.SearchTracesInput{StartTimeMin: "-1h", ServiceName: "test", DurationMin: "-5s"},
			wantErr: "cannot be negative",
		},
		{
			name:    "negative duration_max",
			input:   types.SearchTracesInput{StartTimeMin: "-1h", ServiceName: "test", DurationMax: "-10s"},
			wantErr: "cannot be negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
			require.NoError(t, err)
			handler := &searchTracesHandler{
				queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
				maxResults:   100,
			}

			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, tt.input)

			require.NoError(t, err)
			assert.Empty(t, output.Traces)
			assert.Contains(t, output.Error, tt.wantErr)
			assert.NotContains(t, output.Error, "partial results")
		})
	}
}

// TestSearchTracesHandler_Handle_ZeroDurationValid covers the boundary the negative check
// must not overreach: a zero duration is a valid "no lower/upper bound" sentinel elsewhere
// in this handler (see the durationMin > 0 && durationMax > 0 guard below), so it must still
// reach the query service rather than being rejected as negative.
func TestSearchTracesHandler_Handle_ZeroDurationValid(t *testing.T) {
	want := makeTraceSummary("test", "/test", false)
	mock := newMockFindTraceSummaries(want)

	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMin:  "0s",
		DurationMax:  "10s",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
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

func TestSearchTracesHandler_Handle_SQL(t *testing.T) {
	want := makeTraceSummary("checkout", "/pay", false)
	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			require.NotNil(t, query.Filter)
			assert.Empty(t, query.ServiceName)
			assert.False(t, query.StartTimeMin.IsZero())
			assert.False(t, query.StartTimeMax.IsZero())
			assert.EqualValues(t, 5, query.SearchDepth)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}
	handler := &searchTracesHandler{queryService: mock, maxResults: 100}

	input := types.SearchTracesInput{
		SQL: `
			SELECT DISTINCT trace_id
			FROM spans
			WHERE between(start_time, now() - '1h', now())
				AND service = 'checkout'
			LIMIT 5`,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
	assert.Equal(t, "checkout", output.Traces[0].RootService)
}

// TestSearchTracesHandler_BuildQuery_BlankSQL covers a blank sql input, which means no
// statement and so leaves the structured inputs in charge.
func TestSearchTracesHandler_BuildQuery_BlankSQL(t *testing.T) {
	handler := &searchTracesHandler{maxResults: 100}
	query, err := handler.buildQuery(types.SearchTracesInput{SQL: " \n\t", ServiceName: "checkout"})
	require.NoError(t, err)
	assert.Equal(t, "checkout", query.ServiceName)
	assert.Nil(t, query.Filter)
}

func TestSearchTracesHandler_BuildSQLQuery_Shapes(t *testing.T) {
	const window = "WHERE between(start_time, now() - '1h', now()) AND service = 'checkout'"
	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "trace IDs",
			sql:  "SELECT DISTINCT trace_id FROM spans " + window,
		},
		{
			name: "whole traces",
			sql:  "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans " + window + ")",
		},
		{
			name: "trace summaries",
			sql:  "SELECT trace_id, root_service FROM traces WHERE trace_id IN (SELECT trace_id FROM spans " + window + ")",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &searchTracesHandler{maxResults: 100}
			query, err := handler.buildQuery(types.SearchTracesInput{SQL: tt.sql})
			require.NoError(t, err)
			require.NotNil(t, query.Filter)
			assert.EqualValues(t, "eq", query.Filter.Op)
		})
	}
}

func TestSearchTracesHandler_BuildSQLQuery_SearchDepth(t *testing.T) {
	const window = "WHERE between(start_time, now() - '1h', now())"
	tests := []struct {
		name        string
		sql         string
		searchDepth uint32
		want        uint32
	}{
		{
			name: "the statement's LIMIT is the depth",
			sql:  "SELECT DISTINCT trace_id FROM spans " + window + " LIMIT 7",
			want: 7,
		},
		{
			name:        "LIMIT overrides search_depth",
			sql:         "SELECT DISTINCT trace_id FROM spans " + window + " LIMIT 7",
			searchDepth: 50,
			want:        7,
		},
		{
			name:        "search_depth applies without LIMIT",
			sql:         "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans " + window + ")",
			searchDepth: 50,
			want:        50,
		},
		{
			name: "the default applies without either",
			sql:  "SELECT DISTINCT trace_id FROM spans " + window,
			want: defaultSearchDepth,
		},
		{
			name: "the server cap applies to LIMIT",
			sql:  "SELECT DISTINCT trace_id FROM spans " + window + " LIMIT 500",
			want: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &searchTracesHandler{maxResults: 100}
			query, err := handler.buildQuery(types.SearchTracesInput{SQL: tt.sql, SearchDepth: tt.searchDepth})
			require.NoError(t, err)
			assert.Equal(t, tt.want, query.SearchDepth)
		})
	}
}

// TestSearchTracesHandler_Handle_SQL_EmptyStatement pins that a statement with no SELECT in
// it, such as a lone semicolon, comes back as an ordinary tool error.
func TestSearchTracesHandler_Handle_SQL_EmptyStatement(t *testing.T) {
	handler := &searchTracesHandler{queryService: &mockQueryService{}, maxResults: 100}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.SearchTracesInput{SQL: ";"})

	require.ErrorContains(t, err, "invalid sql: ")
	require.ErrorContains(t, err, "no statement")
	assert.Empty(t, output.Traces)
}

func TestSearchTracesHandler_BuildSQLQuery_Refusals(t *testing.T) {
	const statement = "SELECT DISTINCT trace_id FROM spans WHERE between(start_time, now() - '1h', now())"
	tests := []struct {
		name    string
		input   types.SearchTracesInput
		wantErr string
	}{
		{
			name:    "a span search",
			input:   types.SearchTracesInput{SQL: "SELECT * FROM spans WHERE between(start_time, now() - '1h', now())"},
			wantErr: "invalid sql: the statement selects spans, and search_traces returns traces. Send it to find_spans instead",
		},
		{
			name: "a compile error carries the position and the hint",
			input: types.SearchTracesInput{SQL: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE service = 'checkout'`},
			wantErr: "invalid sql: line 4:5: WHERE needs both bounds on start_time. Add both bounds to WHERE",
		},
		{
			name:    "a parse error",
			input:   types.SearchTracesInput{SQL: "SELEC trace_id FROM spans"},
			wantErr: "invalid sql: line 1:1:",
		},
		{
			name:    "one structured input beside sql",
			input:   types.SearchTracesInput{SQL: statement, ServiceName: "checkout"},
			wantErr: "sql replaces the other filter inputs; remove service_name or write the condition into the statement",
		},
		{
			name: "every structured input beside sql, in the order of the schema",
			input: types.SearchTracesInput{
				SQL:          statement,
				StartTimeMin: "-1h",
				StartTimeMax: "now",
				ServiceName:  "checkout",
				SpanName:     "/pay",
				Attributes:   map[string]string{"error": "true"},
				WithErrors:   true,
				DurationMin:  "1s",
				DurationMax:  "2s",
			},
			wantErr: "remove start_time_min, start_time_max, service_name, span_name, attributes, with_errors, duration_min, duration_max or",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewSearchTracesHandler(nil, 100)
			_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, tt.input)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestSearchTracesHandler_Handle_SQL_UsingMemoryStore is the exit test of RFC 0019 M1: a
// statement returns the traces the equivalent structured inputs return.
func TestSearchTracesHandler_Handle_SQL_UsingMemoryStore(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace("trace111", "checkout", "/ok", false)))
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace("trace222", "checkout", "/error", true)))
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace("trace333", "frontend", "/home", false)))
	handler := &searchTracesHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	tests := []struct {
		name       string
		structured types.SearchTracesInput
		sql        string
		wantSpans  []string
	}{
		{
			name:       "by service, as trace IDs",
			structured: types.SearchTracesInput{ServiceName: "checkout"},
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE between(start_time, now() - '1h', now())
					AND service = 'checkout'`,
			wantSpans: []string{"/error", "/ok"},
		},
		{
			name:       "by service and error, as whole traces",
			structured: types.SearchTracesInput{ServiceName: "checkout", WithErrors: true},
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE between(start_time, now() - '1h', now())
						AND service = 'checkout'
						AND status = 'error'
				)`,
			wantSpans: []string{"/error"},
		},
		{
			name:       "a service without traces",
			structured: types.SearchTracesInput{ServiceName: "payments"},
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE between(start_time, now() - '1h', now())
					AND service = 'payments'`,
			wantSpans: nil,
		},
	}
	rootSpans := func(output types.SearchTracesOutput) []string {
		var names []string
		for _, trace := range output.Traces {
			names = append(names, trace.RootSpanName)
		}
		slices.Sort(names)
		return names
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, viaStructured, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, tt.structured)
			require.NoError(t, err)
			require.Empty(t, viaStructured.Error)
			_, viaSQL, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.SearchTracesInput{SQL: tt.sql})
			require.NoError(t, err)
			require.Empty(t, viaSQL.Error)

			assert.Equal(t, tt.wantSpans, rootSpans(viaStructured))
			assert.Equal(t, tt.wantSpans, rootSpans(viaSQL))
		})
	}
}
