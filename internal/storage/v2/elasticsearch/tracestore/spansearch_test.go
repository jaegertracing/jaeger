// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/mocks"
)

func spanQuery() tracestore.SpanQueryParams {
	start := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	return tracestore.SpanQueryParams{
		StartTimeMin: start,
		StartTimeMax: start.Add(time.Hour),
		Filter:       (builder.Predicate{}).Resource().Service.Eq("test-service"),
		Pagination:   tracestore.Pagination{PageSize: 2},
	}
}

func collectSpanPage(t *testing.T, seq func(func(tracestore.PageChunk[ptrace.Traces], error) bool)) (tracestore.PageChunk[ptrace.Traces], error) {
	var chunks []tracestore.PageChunk[ptrace.Traces]
	var errs []error
	for chunk, err := range seq {
		chunks = append(chunks, chunk)
		errs = append(errs, err)
	}
	require.Len(t, chunks, 1, "a span search yields one page")
	return chunks[0], errs[0]
}

func TestTraceReader_FindSpans(t *testing.T) {
	query := spanQuery()
	query.OrderBy = []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration}}}
	effective, err := tracestore.EffectiveSpanOrder(query.OrderBy)
	require.NoError(t, err)
	spans := []dbmodel.Span{
		{TraceID: "00000000000000000000000000000001", SpanID: "0000000000000001", Process: dbmodel.Process{ServiceName: "a"}},
		{TraceID: "00000000000000000000000000000002", SpanID: "0000000000000002", Process: dbmodel.Process{ServiceName: "b"}},
	}
	coreReader := &mocks.Reader{}
	coreReader.On("FindSpans", mock.Anything, mock.MatchedBy(func(q dbmodel.SpanQueryParameters) bool {
		return q.PageSize == 2 && q.Cursor == nil && assert.Equal(t, effective, q.OrderBy) && q.Filter == query.Filter
	})).Return(dbmodel.SpanPage{Spans: spans, NextCursor: []byte(`[1,"d"]`)}, nil).Once()
	reader := TraceReader{spanReader: coreReader}

	first, err := collectSpanPage(t, reader.FindSpans(context.Background(), query))
	require.NoError(t, err)
	require.Equal(t, 2, first.Results.ResourceSpans().Len(), "every span keeps its own resource, so the page keeps its order")
	assert.Equal(t, pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1}, first.Results.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).SpanID())
	require.NotEmpty(t, first.NextPageToken)

	// The token is handed back as the core reader's cursor, and the last page carries none.
	coreReader.On("FindSpans", mock.Anything, mock.MatchedBy(func(q dbmodel.SpanQueryParameters) bool {
		return string(q.Cursor) == `[1,"d"]`
	})).Return(dbmodel.SpanPage{Spans: spans[:1]}, nil).Once()
	query.Pagination.PageToken = first.NextPageToken
	second, err := collectSpanPage(t, reader.FindSpans(context.Background(), query))
	require.NoError(t, err)
	assert.Equal(t, 1, second.Results.ResourceSpans().Len())
	assert.Empty(t, second.NextPageToken)

	// A page size past the maximum is clamped to it.
	coreReader.On("FindSpans", mock.Anything, mock.MatchedBy(func(q dbmodel.SpanQueryParameters) bool {
		return q.PageSize == int(tracestore.MaxPageSize)
	})).Return(dbmodel.SpanPage{}, nil).Once()
	oversized := spanQuery()
	oversized.Pagination.PageSize = tracestore.MaxPageSize + 1
	_, err = collectSpanPage(t, reader.FindSpans(context.Background(), oversized))
	require.NoError(t, err)

	// An equivalent explicit default order shares the fingerprint, so the token still applies.
	query.OrderBy = append(query.OrderBy, tracestore.SpanSortOrder{
		Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldStartTime},
		Direction:  tracestore.SortDescending,
	})
	coreReader.On("FindSpans", mock.Anything, mock.Anything).Return(dbmodel.SpanPage{}, nil).Once()
	_, err = collectSpanPage(t, reader.FindSpans(context.Background(), query))
	require.NoError(t, err)
	coreReader.AssertExpectations(t)
}

func TestTraceReader_FindSpans_Errors(t *testing.T) {
	token, err := tracestore.NewPageToken([]byte("another query"), []byte("cursor"))
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		query func(q *tracestore.SpanQueryParams)
		core  error
		want  error
		wants string
	}{
		{name: "InvalidOrder", query: func(q *tracestore.SpanQueryParams) {
			q.OrderBy = []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "name"}}}
		}, want: tracestore.ErrSpanOrderInvalid},
		{name: "UnfingerprintableFilter", query: func(q *tracestore.SpanQueryParams) {
			q.Filter = &expression.Call{Op: expression.OpEq, Args: []expression.Expression{(*expression.AttributeRef)(nil)}}
		}, wants: "fingerprint"},
		{name: "TokenOfAnotherQuery", query: func(q *tracestore.SpanQueryParams) {
			q.Pagination.PageToken = token
		}, want: tracestore.ErrPaginationInvalid, wants: "different query"},
		{name: "CoreError", query: func(*tracestore.SpanQueryParams) {}, core: errors.New("search failed"), wants: "search failed"},
		{name: "MalformedSpan", query: func(*tracestore.SpanQueryParams) {}, wants: "encoding/hex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			coreReader := &mocks.Reader{}
			if tc.core != nil {
				coreReader.On("FindSpans", mock.Anything, mock.Anything).Return(dbmodel.SpanPage{}, tc.core)
			} else {
				coreReader.On("FindSpans", mock.Anything, mock.Anything).Return(dbmodel.SpanPage{Spans: []dbmodel.Span{{TraceID: "not hex"}}}, nil)
			}
			query := spanQuery()
			tc.query(&query)
			_, err := collectSpanPage(t, (&TraceReader{spanReader: coreReader}).FindSpans(context.Background(), query))
			require.Error(t, err)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}
			if tc.wants != "" {
				require.ErrorContains(t, err, tc.wants)
			}
			if tc.core == nil && tc.wants != "encoding/hex" {
				coreReader.AssertNotCalled(t, "FindSpans")
			}
		})
	}
}

// TestTraceReader_TraceSearchesUnderPagination pins how the trace searches, which do not page
// yet, honor the Pagination the query service passes them once the reader declares Paginated:
// the page size bounds the search and a token is refused.
func TestTraceReader_TraceSearchesUnderPagination(t *testing.T) {
	ts := time.Now()
	query := tracestore.TraceQueryParams{
		ServiceName:  "svc",
		Attributes:   pcommon.NewMap(),
		StartTimeMin: ts,
		StartTimeMax: ts.Add(time.Hour),
		Pagination:   &tracestore.Pagination{PageSize: 7},
	}
	coreReader := &mocks.Reader{}
	coreReader.On("FindTraceIDs", mock.Anything, mock.MatchedBy(func(q dbmodel.TraceQueryParameters) bool {
		return q.SearchDepth == 7
	})).Return([]dbmodel.TraceID{}, nil)
	coreReader.On("FindTraceSummaries", mock.Anything, mock.MatchedBy(func(q dbmodel.TraceQueryParameters) bool {
		return q.SearchDepth == 7
	})).Return([]dbmodel.TraceSummary{}, nil)
	reader := TraceReader{spanReader: coreReader}
	for chunk, err := range reader.FindTraceIDs(context.Background(), query) {
		require.NoError(t, err)
		assert.Empty(t, chunk.NextPageToken)
	}
	for chunk, err := range reader.FindTraceSummaries(context.Background(), query) {
		require.NoError(t, err)
		assert.Empty(t, chunk.NextPageToken)
	}
	coreReader.AssertExpectations(t)

	query.Pagination.PageToken = "some-token"
	var refusals int
	for _, err := range reader.FindTraceIDs(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationUnsupported)
		refusals++
	}
	for _, err := range reader.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationUnsupported)
		refusals++
	}
	assert.Equal(t, 2, refusals, "each trace search must yield the refusal rather than nothing")
}
