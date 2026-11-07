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

const findTracesWindow = "WHERE between(start_time, now() - '1h', now())"

func TestFindTracesHandler_Handle_FullWorkflow(t *testing.T) {
	want := makeTraceSummary("checkout", "/pay", false)
	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			require.NotNil(t, query.Filter)
			assert.EqualValues(t, "eq", query.Filter.Op)
			assert.Empty(t, query.ServiceName, "the statement's predicates travel in the filter, not the legacy fields")
			assert.WithinDuration(t, time.Now().Add(-time.Hour), query.StartTimeMin, 5*time.Second)
			assert.WithinDuration(t, time.Now(), query.StartTimeMax, 5*time.Second)
			assert.EqualValues(t, 5, query.SearchDepth)
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield([]tracestore.TraceSummary{want}, nil)
			}
		},
	}
	handler := &findTracesHandler{queryService: mock, maxResults: 100}

	input := types.FindTracesInput{
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
	assert.Equal(t, "/pay", output.Traces[0].RootSpanName)
	assert.Empty(t, output.Error)
}

func TestFindTracesHandler_Handle_PartialResults(t *testing.T) {
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
	handler := &findTracesHandler{queryService: mock, maxResults: 100}

	input := types.FindTracesInput{SQL: "SELECT DISTINCT trace_id FROM spans " + findTracesWindow}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Traces, 1)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "temporary failure")
}

// TestFindTracesHandler_Handle_ErrorBeforeAnyResult covers a search that fails before
// any summary came back: the call fails with that error instead of returning an empty page.
func TestFindTracesHandler_Handle_ErrorBeforeAnyResult(t *testing.T) {
	refused := errors.New("backend refused the query")
	mock := &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield(nil, refused)
			}
		},
	}
	handler := &findTracesHandler{queryService: mock, maxResults: 100}

	input := types.FindTracesInput{SQL: "SELECT DISTINCT trace_id FROM spans " + findTracesWindow}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.ErrorIs(t, err, refused)
	assert.Empty(t, output.Traces)
}

// TestFindTracesHandler_Handle_LimitEnforced covers the server's cap on a page: the handler
// stops reading summaries once the cap is reached, whatever the backend keeps yielding.
func TestFindTracesHandler_Handle_LimitEnforced(t *testing.T) {
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
	handler := &findTracesHandler{queryService: mock, maxResults: 3}

	input := types.FindTracesInput{SQL: "SELECT DISTINCT trace_id FROM spans " + findTracesWindow}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Traces, 3)
}

func TestFindTracesHandler_BuildQuery_Shapes(t *testing.T) {
	const window = findTracesWindow + " AND service = 'checkout'"
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
			handler := &findTracesHandler{maxResults: 100}
			query, err := handler.buildQuery(types.FindTracesInput{SQL: tt.sql})
			require.NoError(t, err)
			require.NotNil(t, query.Filter)
			assert.EqualValues(t, "eq", query.Filter.Op)
		})
	}
}

func TestFindTracesHandler_BuildQuery_SearchDepth(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		maxResults uint32
		want       uint32
	}{
		{
			name:       "the statement's LIMIT is the depth",
			sql:        "SELECT DISTINCT trace_id FROM spans " + findTracesWindow + " LIMIT 7",
			maxResults: 100,
			want:       7,
		},
		{
			name:       "the outer LIMIT of a traces query is the depth",
			sql:        "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans " + findTracesWindow + ") LIMIT 7",
			maxResults: 100,
			want:       7,
		},
		{
			name:       "the default applies without LIMIT",
			sql:        "SELECT DISTINCT trace_id FROM spans " + findTracesWindow,
			maxResults: 100,
			want:       defaultSearchDepth,
		},
		{
			name:       "the server cap applies to LIMIT",
			sql:        "SELECT DISTINCT trace_id FROM spans " + findTracesWindow + " LIMIT 500",
			maxResults: 100,
			want:       100,
		},
		{
			name:       "no server cap leaves LIMIT alone",
			sql:        "SELECT DISTINCT trace_id FROM spans " + findTracesWindow + " LIMIT 500",
			maxResults: 0,
			want:       500,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &findTracesHandler{maxResults: tt.maxResults}
			query, err := handler.buildQuery(types.FindTracesInput{SQL: tt.sql})
			require.NoError(t, err)
			assert.Equal(t, tt.want, query.SearchDepth)
		})
	}
}

// TestFindTracesHandler_Handle_Refusals covers the refusals that are the handler's own: a
// missing statement, a statement with no SELECT in it, a statement of a shape this tool does
// not serve, and the prefix it puts on a compiler error. What the compiler refuses, and how,
// is the sqlquery package's to test.
func TestFindTracesHandler_Handle_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr []string
	}{
		{
			name:    "no statement",
			sql:     " \n\t",
			wantErr: []string{"sql is required"},
		},
		{
			name:    "a lone semicolon",
			sql:     ";",
			wantErr: []string{"invalid sql: ", "no statement"},
		},
		{
			name:    "a span search",
			sql:     "SELECT * FROM spans " + findTracesWindow,
			wantErr: []string{"invalid sql: find_traces takes one of: SELECT DISTINCT trace_id FROM spans WHERE"},
		},
		{
			name:    "a compiler error carries the invalid sql prefix",
			sql:     "SELECT DISTINCT trace_id FROM spans WHERE service = 'checkout'",
			wantErr: []string{"invalid sql: line 1:37:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewFindTracesHandler(nil, 100)
			_, output, err := handler(context.Background(), &mcp.CallToolRequest{}, types.FindTracesInput{SQL: tt.sql})
			for _, want := range tt.wantErr {
				require.ErrorContains(t, err, want)
			}
			assert.Empty(t, output.Traces)
		})
	}
}

// TestFindTracesHandler_Handle_UsingMemoryStore runs statements of both shapes through the
// query service against the memory backend and checks the traces they select.
func TestFindTracesHandler_Handle_UsingMemoryStore(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace("trace111", "checkout", "/ok", false)))
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace("trace222", "checkout", "/error", true)))
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace("trace333", "frontend", "/home", false)))
	handler := &findTracesHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	tests := []struct {
		name      string
		sql       string
		wantSpans []string
	}{
		{
			name: "by service, as trace IDs",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE between(start_time, now() - '1h', now())
					AND service = 'checkout'`,
			wantSpans: []string{"/error", "/ok"},
		},
		{
			name: "by service and error, as whole traces",
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
			name: "across every service",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE between(start_time, now() - '1h', now())
					AND status <> 'error'`,
			wantSpans: []string{"/home", "/ok"},
		},
		{
			name: "a service without traces",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE between(start_time, now() - '1h', now())
					AND service = 'payments'`,
			wantSpans: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, types.FindTracesInput{SQL: tt.sql})
			require.NoError(t, err)
			require.Empty(t, output.Error)
			var names []string
			for _, trace := range output.Traces {
				names = append(names, trace.RootSpanName)
			}
			slices.Sort(names)
			assert.Equal(t, tt.wantSpans, names)
		})
	}
}
