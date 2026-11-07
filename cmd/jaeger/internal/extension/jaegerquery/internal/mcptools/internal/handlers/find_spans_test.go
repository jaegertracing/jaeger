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
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
)

const findSpansWindow = "WHERE between(start_time, now() - '1h', now())"

func TestFindSpansHandler_Handle_FullWorkflow(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/checkout"},
		{spanID: "span0002", parentSpanID: "span0001", operation: "/charge"},
	})

	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			assert.False(t, query.StartTimeMin.IsZero())
			assert.False(t, query.StartTimeMax.IsZero())
			require.NotNil(t, query.Filter)
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				yield(tracestore.PageChunk[ptrace.Traces]{Results: trace}, nil)
			}
		},
	}

	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{
		SQL: "SELECT * FROM spans " + findSpansWindow + " AND service = 'test-service' AND name = '/checkout'",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 2)
	assert.Equal(t, "/checkout", output.Spans[0].SpanName)
	assert.Empty(t, output.Spans[0].ParentSpanID)
	assert.Equal(t, "/charge", output.Spans[1].SpanName)
	assert.Equal(t, output.Spans[0].SpanID, output.Spans[1].ParentSpanID)
	assert.Equal(t, output.Spans[0].TraceID, output.Spans[1].TraceID)
	assert.NotEmpty(t, output.Spans[1].TraceID)
	assert.Equal(t, "test-service", output.Spans[1].Service)
	assert.Empty(t, output.Error)
}

func TestFindSpansHandler_Handle_WindowOnlyHasNoFilter(t *testing.T) {
	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			assert.Nil(t, query.Filter, "a statement with only the window compiles to no filter")
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				yield(tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()}, nil)
			}
		},
	}
	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)
}

func TestFindSpansHandler_Handle_BlankSQL(t *testing.T) {
	// Via the exported constructor, like the validation-error tests of NewFindTracesHandler
	// and NewGetSpanDetailsHandler: nil is a valid *querysvc.QueryService here because this
	// path returns before ever touching it.
	handler := NewFindSpansHandler(nil, 100)
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, types.FindSpansInput{SQL: " \n\t"})
	require.ErrorContains(t, err, "sql is required")
}

func TestFindSpansHandler_Handle_QueryError(t *testing.T) {
	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, _ querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				yield(tracestore.PageChunk[ptrace.Traces]{}, errors.New("backend unavailable"))
			}
		},
	}
	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.ErrorContains(t, err, "backend unavailable", "a search that returns nothing fails the call")
	assert.Empty(t, output.Spans)
}

func TestFindSpansHandler_Handle_SpanSearchUnsupported(t *testing.T) {
	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, _ querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				yield(tracestore.PageChunk[ptrace.Traces]{}, querysvc.ErrSpanSearchUnsupported)
			}
		},
	}
	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.ErrorIs(t, err, querysvc.ErrSpanSearchUnsupported)
	assert.Empty(t, output.Spans)
}

func TestFindSpansHandler_Handle_PartialResults(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/first"},
	})
	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, _ querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				if !yield(tracestore.PageChunk[ptrace.Traces]{Results: trace}, nil) {
					return
				}
				yield(tracestore.PageChunk[ptrace.Traces]{}, errors.New("connection lost"))
			}
		},
	}
	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.Contains(t, output.Error, "partial results")
	assert.Contains(t, output.Error, "connection lost")
}

func TestFindSpansHandler_Handle_LimitEnforced(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/a"},
		{spanID: "span0002", operation: "/b"},
		{spanID: "span0003", operation: "/c"},
	})
	mock := newMockFindSpans(trace)
	handler := &findSpansHandler{queryService: mock, maxResults: 2}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 2)
}

// TestFindSpansHandler_BuildQuery_EffectiveLimit pins how the statement's LIMIT and the server
// cap interact, and that the result is also the page size the backend is asked for, so a
// request for one span does not make the backend fetch a full default page.
func TestFindSpansHandler_BuildQuery_EffectiveLimit(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		cap  uint32
		want uint32
	}{
		{
			name: "the statement's LIMIT is the limit",
			sql:  "SELECT * FROM spans " + findSpansWindow + " LIMIT 1",
			cap:  100,
			want: 1,
		},
		{
			name: "the server cap stands in for an absent LIMIT",
			sql:  "SELECT * FROM spans " + findSpansWindow,
			cap:  100,
			want: 100,
		},
		{
			name: "the server cap bounds LIMIT",
			sql:  "SELECT * FROM spans " + findSpansWindow + " LIMIT 500",
			cap:  100,
			want: 100,
		},
		{
			name: "without a cap, LIMIT passes through",
			sql:  "SELECT * FROM spans " + findSpansWindow + " LIMIT 7",
			want: 7,
		},
		{
			name: "without a cap or LIMIT, zero leaves the default to the query service",
			sql:  "SELECT * FROM spans " + findSpansWindow,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &findSpansHandler{maxResults: tt.cap}
			query, limit, err := handler.buildQuery(types.FindSpansInput{SQL: tt.sql})
			require.NoError(t, err)
			assert.Equal(t, tt.want, limit)
			assert.Equal(t, tt.want, query.Pagination.PageSize)
		})
	}
}

// TestFindSpansHandler_BuildQuery_Refusals covers the two refusals that are the handler's
// own: a statement of a shape this tool does not serve, and the prefix it puts on a compiler
// error. What the compiler refuses, and how, is the sqlquery package's to test.
func TestFindSpansHandler_BuildQuery_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr string
	}{
		{
			name:    "a trace-ID search",
			sql:     "SELECT DISTINCT trace_id FROM spans " + findSpansWindow,
			wantErr: "invalid sql: the statement returns traces, and find_spans returns spans",
		},
		{
			name:    "a compiler error carries the invalid sql prefix",
			sql:     "SELECT * FROM spans WHERE service = 'checkout'",
			wantErr: "invalid sql: line 1:21:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewFindSpansHandler(nil, 100)
			_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, types.FindSpansInput{SQL: tt.sql})
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestFindSpansHandler_Handle_ServedByMemoryStore pins the end-to-end behavior against the
// built-in memory backend, which declares SearchCapabilities.SpanSearch (RFC 0016 M2), so
// find_spans returns the stored span rather than refusing.
func TestFindSpansHandler_Handle_ServedByMemoryStore(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace(
		"trace111", "test", "/ok", false,
	)))

	handler := &findSpansHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, "/ok", output.Spans[0].SpanName)
	assert.Empty(t, output.Error)
}

// A column list reaches the query service as a projection, which refuses it until a result
// shaped by the column list exists. The tool passes the refusal through as the call's error.
func TestFindSpansHandler_Handle_ColumnListRefusedByQueryService(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)

	handler := &findSpansHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.FindSpansInput{SQL: "SELECT service, name FROM spans " + findSpansWindow}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.ErrorIs(t, err, querysvc.ErrProjectionUnsupported)
	assert.Empty(t, output.Spans)
}

func TestFindSpansHandler_Handle_StatusFilter(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/error", hasError: true, errorMessage: "boom"},
	})

	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	require.NoError(t, store.WriteTraces(context.Background(), trace))

	handler := &findSpansHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow + " AND status = 'error'"}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, "/error", output.Spans[0].SpanName)
	assert.Equal(t, "Error", output.Spans[0].Status)
	assert.Empty(t, output.Error)
}
