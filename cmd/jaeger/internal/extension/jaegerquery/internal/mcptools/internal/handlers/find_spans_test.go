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

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
)

func TestFindSpansHandler_Handle_FullWorkflow(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/checkout"},
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
		StartTimeMin: "-1h",
		StartTimeMax: "now",
		ServiceName:  "test-service",
		SpanName:     "/checkout",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, "/checkout", output.Spans[0].SpanName)
	assert.Empty(t, output.Error)
}

func TestFindSpansHandler_Handle_NoFilterWhenNoCriteria(t *testing.T) {
	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			assert.Nil(t, query.Filter, "no criteria means the base case: time range alone")
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				yield(tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()}, nil)
			}
		},
	}
	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now"}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)
}

func TestFindSpansHandler_Handle_MissingStartTimeMin(t *testing.T) {
	// Via the exported constructor, like NewSearchTracesHandler/NewGetSpanDetailsHandler's own
	// validation-error tests: nil is a valid *querysvc.QueryService here because this path
	// returns before ever touching it.
	handler := NewFindSpansHandler(nil, 100)
	input := types.FindSpansInput{StartTimeMax: "now"}
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)
	require.ErrorContains(t, err, "start_time_min is required")
}

func TestFindSpansHandler_Handle_MissingStartTimeMax(t *testing.T) {
	handler := &findSpansHandler{queryService: &mockQueryService{}, maxResults: 100}
	input := types.FindSpansInput{StartTimeMin: "-1h"}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.ErrorContains(t, err, "start_time_max is required")
}

func TestFindSpansHandler_Handle_InvalidStartTimeMin(t *testing.T) {
	handler := &findSpansHandler{queryService: &mockQueryService{}, maxResults: 100}
	input := types.FindSpansInput{StartTimeMin: "not-a-time", StartTimeMax: "now"}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.ErrorContains(t, err, "invalid start_time_min")
}

func TestFindSpansHandler_Handle_InvalidStartTimeMax(t *testing.T) {
	handler := &findSpansHandler{queryService: &mockQueryService{}, maxResults: 100}
	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "not-a-time"}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.ErrorContains(t, err, "invalid start_time_max")
}

func TestFindSpansHandler_Handle_InvalidDurationMin(t *testing.T) {
	handler := &findSpansHandler{queryService: &mockQueryService{}, maxResults: 100}
	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now", DurationMin: "nope"}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.ErrorContains(t, err, "invalid duration_min")
}

func TestFindSpansHandler_Handle_InvalidDurationMax(t *testing.T) {
	handler := &findSpansHandler{queryService: &mockQueryService{}, maxResults: 100}
	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now", DurationMax: "nope"}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.ErrorContains(t, err, "invalid duration_max")
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

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now"}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err, "a query error is reported in output.Error, not returned")
	assert.Empty(t, output.Spans)
	assert.Contains(t, output.Error, "backend unavailable")
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

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now"}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Empty(t, output.Spans)
	assert.Contains(t, output.Error, querysvc.ErrSpanSearchUnsupported.Error())
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

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now"}
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

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now"}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 2)
}

func TestFindSpansHandler_Handle_MaxResultsClampedToServerCap(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/a"},
		{spanID: "span0002", operation: "/b"},
		{spanID: "span0003", operation: "/c"},
	})
	mock := newMockFindSpans(trace)
	handler := &findSpansHandler{queryService: mock, maxResults: 2}

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now", MaxResults: 100}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 2, "a request above the server cap is clamped to it")
}

func TestFindSpansHandler_Handle_WithErrorsFilter(t *testing.T) {
	trace := createTestTraceWithSpans("trace111", []spanConfig{
		{spanID: "span0001", operation: "/error", hasError: true, errorMessage: "boom"},
	})

	mock := &mockQueryService{
		findSpansFunc: func(_ context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
			require.NotNil(t, query.Filter)
			ref, ok := query.Filter.Args[0].(*expression.AttributeRef)
			require.True(t, ok)
			assert.Equal(t, "error", ref.Key)
			return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
				yield(tracestore.PageChunk[ptrace.Traces]{Results: trace}, nil)
			}
		},
	}
	handler := &findSpansHandler{queryService: mock, maxResults: 100}

	input := types.FindSpansInput{
		StartTimeMin: "-1h",
		StartTimeMax: "now",
		WithErrors:   true,
	}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, "/error", output.Spans[0].SpanName)
	assert.Equal(t, "Error", output.Spans[0].Status.Code)
	assert.Empty(t, output.Error)
}

// TestFindSpansHandler_Handle_SpanSearchUnsupportedOnMemoryStore pins today's actual
// end-to-end behavior against the built-in memory backend: no backend on main declares
// SearchCapabilities.SpanSearch yet (RFC 0016 M3 through M6 are still landing), so find_spans
// refuses cleanly rather than returning nothing silently or panicking.
func TestFindSpansHandler_Handle_SpanSearchUnsupportedOnMemoryStore(t *testing.T) {
	store, err := memory.NewStore(memory.Configuration{MaxTraces: 10})
	require.NoError(t, err)
	require.NoError(t, store.WriteTraces(context.Background(), createTestTrace(
		"trace111", "test", "/ok", false,
	)))

	handler := &findSpansHandler{
		queryService: querysvc.NewQueryService(store, store, querysvc.QueryServiceOptions{}),
		maxResults:   100,
	}

	input := types.FindSpansInput{StartTimeMin: "-1h", StartTimeMax: "now"}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Empty(t, output.Spans)
	assert.Contains(t, output.Error, querysvc.ErrSpanSearchUnsupported.Error())
}

func TestBuildSearchFilter(t *testing.T) {
	t.Run("no criteria yields nil", func(t *testing.T) {
		assert.Nil(t, buildSearchFilter(types.FindSpansInput{}, 0, 0))
	})
	t.Run("single criterion is not wrapped in and", func(t *testing.T) {
		f := buildSearchFilter(types.FindSpansInput{ServiceName: "svc"}, 0, 0)
		require.NotNil(t, f)
		assert.Equal(t, expression.OpEq, f.Op)
	})
	t.Run("multiple criteria are anded together", func(t *testing.T) {
		f := buildSearchFilter(types.FindSpansInput{ServiceName: "svc", SpanName: "op"}, 0, 0)
		require.NotNil(t, f)
		assert.Equal(t, expression.OpAnd, f.Op)
		assert.Len(t, f.Args, 2)
	})
	t.Run("attribute value is untyped so it resolves against the stored kind", func(t *testing.T) {
		f := buildSearchFilter(types.FindSpansInput{Attributes: map[string]string{"k": "v"}}, 0, 0)
		require.NotNil(t, f)
		call, ok := f.Args[1].(*expression.AnyValue)
		require.True(t, ok)
		assert.Equal(t, "v", call.Value)
	})
	t.Run("with_errors reads the error virtual attribute as a bool", func(t *testing.T) {
		f := buildSearchFilter(types.FindSpansInput{WithErrors: true}, 0, 0)
		require.NotNil(t, f)
		ref, ok := f.Args[0].(*expression.AttributeRef)
		require.True(t, ok)
		assert.Equal(t, "error", ref.Key)
		value, ok := f.Args[1].(*expression.BoolValue)
		require.True(t, ok)
		assert.True(t, value.Value)
	})
	t.Run("duration bounds compare the span duration field", func(t *testing.T) {
		f := buildSearchFilter(types.FindSpansInput{}, 2*time.Second, 10*time.Second)
		require.NotNil(t, f)
		require.Equal(t, expression.OpAnd, f.Op)
		require.Len(t, f.Args, 2)
		lower := f.Args[0].(*expression.Call)
		assert.Equal(t, expression.OpGte, lower.Op)
		upper := f.Args[1].(*expression.Call)
		assert.Equal(t, expression.OpLte, upper.Op)
	})
}
