// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// spanSearchRow is a span row with the fields a span search sorts on and one attribute that
// tells two rows with the same key apart.
func spanSearchRow(name, spanID string, start time.Time) *dbmodel.SpanRow {
	return &dbmodel.SpanRow{
		ID:         spanID,
		TraceID:    traceIDHex1,
		Name:       name,
		Kind:       "server",
		StartTime:  start,
		StatusCode: "Ok",
		Duration:   1000,
		Attributes: dbmodel.Attributes{
			StrKeys:   []string{"row"},
			StrValues: []string{name},
		},
	}
}

// spanSearchQuery is the span query the tests run: a time range around now and a filter on the
// service.
func spanSearchQuery(pageSize uint32) tracestore.SpanQueryParams {
	return tracestore.SpanQueryParams{
		StartTimeMin: now.Add(-time.Hour),
		StartTimeMax: now,
		Filter:       call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("cart")),
		Pagination:   tracestore.Pagination{PageSize: pageSize},
	}
}

// spanSearchConn returns a connection whose span search answers with the given rows. The rows
// of a later call are swapped in with serve.
func spanSearchConn(rows ...*dbmodel.SpanRow) (conn *clickhousetest.Driver, serve func(rows ...*dbmodel.SpanRow)) {
	response := &clickhousetest.QueryResponse{}
	serve = func(rows ...*dbmodel.SpanRow) {
		response.Rows = &clickhousetest.Rows[*dbmodel.SpanRow]{Data: rows, ScanFn: scanSpanRowFn()}
	}
	serve(rows...)
	conn = &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansQuery: response,
		},
	}
	return conn, serve
}

// findSpans runs one page and returns the names of its spans in order with the next token.
func findSpans(t *testing.T, reader *Reader, query tracestore.SpanQueryParams) ([]string, tracestore.PageToken) {
	t.Helper()
	var names []string
	var token tracestore.PageToken
	pages := 0
	for chunk, err := range reader.FindSpans(context.Background(), query) {
		require.NoError(t, err)
		pages++
		for _, span := range jptrace.SpanIter(chunk.Results) {
			names = append(names, span.Name())
		}
		token = chunk.NextPageToken
	}
	require.Equal(t, 1, pages, "a page is one chunk")
	return names, token
}

func findSpansError(reader *Reader, query tracestore.SpanQueryParams) error {
	for _, err := range reader.FindSpans(context.Background(), query) {
		return err
	}
	return nil
}

func TestFindSpans_Query(t *testing.T) {
	conn, _ := spanSearchConn(
		spanSearchRow("a", "0000000000000001", now),
		spanSearchRow("b", "0000000000000002", now.Add(-time.Second)),
		spanSearchRow("c", "0000000000000003", now.Add(-2*time.Second)),
	)
	reader := NewReader(conn, testReaderConfig)

	names, token := findSpans(t, reader, spanSearchQuery(2))
	assert.Equal(t, []string{"a", "b"}, names, "the reader stops at the page size whatever the statement returned")
	assert.Empty(t, token)

	require.Len(t, conn.RecordedQueries, 1)
	verifyQuerySnapshot(t, conn.RecordedQueries...)
	assert.Equal(t, []any{
		now.Add(-time.Hour).UnixNano(), now.UnixNano(), "cart", uint64(2),
	}, conn.RecordedArgs[0])
}

// TestFindSpans_TimeRangeOnly snapshots the statement of a span query that carries no filter.
func TestFindSpans_TimeRangeOnly(t *testing.T) {
	conn, _ := spanSearchConn(spanSearchRow("a", "0000000000000001", now))
	reader := NewReader(conn, testReaderConfig)
	query := spanSearchQuery(5)
	query.Filter = nil
	names, token := findSpans(t, reader, query)
	assert.Equal(t, []string{"a"}, names)
	assert.Empty(t, token)
	require.Len(t, conn.RecordedQueries, 1)
	verifyQuerySnapshot(t, conn.RecordedQueries...)
	assert.Equal(t, []any{now.Add(-time.Hour).UnixNano(), now.UnixNano(), uint64(5)}, conn.RecordedArgs[0])
}

func TestFindSpans_PageSizeIsClamped(t *testing.T) {
	conn, _ := spanSearchConn()
	reader := NewReader(conn, testReaderConfig)
	names, token := findSpans(t, reader, spanSearchQuery(tracestore.MaxPageSize+1))
	assert.Empty(t, names)
	assert.Empty(t, token)
	require.Len(t, conn.RecordedArgs, 1)
	assert.Equal(t, uint64(tracestore.MaxPageSize), conn.RecordedArgs[0][len(conn.RecordedArgs[0])-1])
}

func TestFindSpans_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		query   tracestore.SpanQueryParams
		wantErr error
		message string
	}{
		{
			name: "explicit order",
			query: func() tracestore.SpanQueryParams {
				q := spanSearchQuery(1)
				q.OrderBy = []tracestore.SpanSortOrder{{
					Expression: fieldRef(expression.LevelSpan, expression.SpanFieldDuration),
					Direction:  tracestore.SortDescending,
				}}
				return q
			}(),
			wantErr: tracestore.ErrSpanOrderUnsupported,
			message: "explicit span ordering",
		},
		{
			name:    "zero page size",
			query:   spanSearchQuery(0),
			wantErr: tracestore.ErrPaginationInvalid,
			message: "page size must be greater than 0",
		},
		{
			name: "unsupported filter",
			query: func() tracestore.SpanQueryParams {
				q := spanSearchQuery(1)
				q.Filter = call(expression.OpRegex, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("a.*"))
				return q
			}(),
			wantErr: tracestore.ErrFilterUnsupported,
			message: "failed to build query",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := spanSearchConn()
			err := findSpansError(NewReader(conn, testReaderConfig), tt.query)
			require.ErrorIs(t, err, tt.wantErr)
			require.ErrorContains(t, err, tt.message)
			assert.Empty(t, conn.RecordedQueries, "a refused query never reaches the database")
		})
	}
}

func TestFindSpans_Errors(t *testing.T) {
	t.Run("query fails", func(t *testing.T) {
		conn, _ := spanSearchConn()
		conn.QueryResponses[sql.SelectSpansQuery].Err = errors.New("boom")
		err := findSpansError(NewReader(conn, testReaderConfig), spanSearchQuery(1))
		require.ErrorContains(t, err, "failed to query spans: boom")
	})
	t.Run("scan fails", func(t *testing.T) {
		conn, _ := spanSearchConn(spanSearchRow("a", "0000000000000001", now))
		conn.QueryResponses[sql.SelectSpansQuery].Rows.(*clickhousetest.Rows[*dbmodel.SpanRow]).ScanErr = errors.New("boom")
		err := findSpansError(NewReader(conn, testReaderConfig), spanSearchQuery(1))
		require.ErrorContains(t, err, "failed to scan span row: boom")
	})
}
