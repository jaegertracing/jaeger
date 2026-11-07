// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"strings"
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

	_, output, err := handler.handle(t.Context(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 2)
	assert.Equal(t, "/checkout", output.Spans[0].SpanName)
	assert.Empty(t, output.Spans[0].ParentSpanID)
	assert.Equal(t, "/charge", output.Spans[1].SpanName)
	assert.Equal(t, output.Spans[0].SpanID, output.Spans[1].ParentSpanID)
	assert.Equal(t, output.Spans[0].TraceID, output.Spans[1].TraceID)
	assert.NotEmpty(t, output.Spans[1].TraceID)
	assert.Equal(t, "test-service", output.Spans[1].Service)
	assert.NotEmpty(t, output.Spans[0].Duration)
	assert.Empty(t, output.Error)
}

func TestFindSpansHandler_Handle_BlankSQL(t *testing.T) {
	handler := NewFindSpansHandler(nil, 100)
	_, _, err := handler(t.Context(), &mcp.CallToolRequest{}, types.FindSpansInput{SQL: " \n\t"})
	require.ErrorContains(t, err, "sql is required")
}

// TestFindSpansHandler_Handle_Results pins what the handler makes of the chunks the query
// service yields: how many spans it keeps, and whether an error fails the call or is reported
// beside the spans collected before it.
func TestFindSpansHandler_Handle_Results(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/a"},
		{spanID: "span0002", operation: "/b"},
		{spanID: "span0003", operation: "/c"},
	})
	chunk := tracestore.PageChunk[ptrace.Traces]{Results: trace}
	searchErr := errors.New("connection lost")

	tests := []struct {
		name      string
		chunks    []tracestore.PageChunk[ptrace.Traces]
		errAfter  error
		cap       uint32
		wantSpans int
		wantErr   error
		wantNote  string
	}{
		{
			name:      "every span of every chunk",
			chunks:    []tracestore.PageChunk[ptrace.Traces]{chunk, chunk},
			cap:       100,
			wantSpans: 6,
		},
		{
			name:      "the limit stops the collection",
			chunks:    []tracestore.PageChunk[ptrace.Traces]{chunk, chunk},
			cap:       2,
			wantSpans: 2,
		},
		{
			name:      "an error after a chunk is reported beside the spans",
			chunks:    []tracestore.PageChunk[ptrace.Traces]{chunk},
			errAfter:  searchErr,
			cap:       100,
			wantSpans: 3,
			wantNote:  "partial results returned due to error: connection lost",
		},
		{
			name:     "an error before any span fails the call",
			errAfter: searchErr,
			cap:      100,
			wantErr:  searchErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockQueryService{
				findSpansFunc: func(_ context.Context, _ querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
					return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
						for _, c := range tt.chunks {
							if !yield(c, nil) {
								return
							}
						}
						if tt.errAfter != nil {
							yield(tracestore.PageChunk[ptrace.Traces]{}, tt.errAfter)
						}
					}
				},
			}
			handler := &findSpansHandler{queryService: mock, maxResults: tt.cap}

			input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
			_, output, err := handler.handle(t.Context(), &mcp.CallToolRequest{}, input)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Len(t, output.Spans, tt.wantSpans)
			assert.Equal(t, tt.wantNote, output.Error)
		})
	}
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

// A column list reaches the query service as the projection of the request, which is the
// query service's to refuse or shape.
func TestFindSpansHandler_BuildQuery_ColumnListIsTheProjection(t *testing.T) {
	handler := &findSpansHandler{maxResults: 100}
	query, _, err := handler.buildQuery(types.FindSpansInput{SQL: "SELECT service, name FROM spans " + findSpansWindow})
	require.NoError(t, err)
	assert.Len(t, query.Projection, 2)
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
			wantErr: "invalid sql: find_spans takes SELECT * FROM spans WHERE",
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
			_, _, err := handler(t.Context(), &mcp.CallToolRequest{}, types.FindSpansInput{SQL: tt.sql})
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestFindSpansHandler_Handle_ServedByMemoryStore is the one test through the real query
// service and a storage backend: the statement compiles into a search the memory store
// serves, which is the exit criterion of RFC 0019 M1a.
func TestFindSpansHandler_Handle_ServedByMemoryStore(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	require.NoError(t, store.WriteTraces(t.Context(), createTestTrace(
		"trace111", "test", "/ok", false,
	)))

	handler := &findSpansHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.FindSpansInput{SQL: "SELECT * FROM spans " + findSpansWindow}
	_, output, err := handler.handle(t.Context(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, "/ok", output.Spans[0].SpanName)
	assert.Empty(t, output.Error)
}

// TestFindSpansHandler_ColumnListSchemaMatchesBehavior ties the NoColumnList sentence in the
// tool's schema to what the query service does with a column list: the schema carries the
// sentence exactly when the query service refuses the list. When sparse spans land, the
// refusal stops and this test fails until the sentence is removed from the schema.
func TestFindSpansHandler_ColumnListSchemaMatchesBehavior(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	handler := &findSpansHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.FindSpansInput{SQL: "SELECT service, name FROM spans " + findSpansWindow}
	_, _, err = handler.handle(t.Context(), &mcp.CallToolRequest{}, input)

	refused := errors.Is(err, querysvc.ErrProjectionUnsupported)
	assert.Equal(t, refused, schemaSaysNoColumnList(t, input),
		"the find_spans schema must say %q exactly when a column list is refused; got %v", types.NoColumnList, err)
}

// schemaSaysNoColumnList reports whether the schema of a tool input's SQL field carries the
// NoColumnList sentence. A struct tag is a string literal and cannot refer to the constant, so
// the tag spells the sentence out and the tests of each tool compare the two.
func schemaSaysNoColumnList(t *testing.T, input any) bool {
	field, ok := reflect.TypeOf(input).FieldByName("SQL")
	require.True(t, ok)
	return strings.Contains(field.Tag.Get("jsonschema"), types.NoColumnList)
}
