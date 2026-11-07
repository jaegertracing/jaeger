// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

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

// spanSearchQuery is the span query the tests page through: a time range around now and a
// filter on the service.
func spanSearchQuery(pageSize uint32, token tracestore.PageToken) tracestore.SpanQueryParams {
	return tracestore.SpanQueryParams{
		StartTimeMin: now.Add(-time.Hour),
		StartTimeMax: now,
		Filter:       call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("cart")),
		Pagination:   tracestore.Pagination{PageSize: pageSize, PageToken: token},
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
	conn, serve := spanSearchConn(
		spanSearchRow("a", "0000000000000001", now),
		spanSearchRow("b", "0000000000000002", now.Add(-time.Second)),
		spanSearchRow("c", "0000000000000003", now.Add(-2*time.Second)),
	)
	reader := NewReader(conn, testReaderConfig)

	names, token := findSpans(t, reader, spanSearchQuery(2, ""))
	assert.Equal(t, []string{"a", "b"}, names)
	require.NotEmpty(t, token)

	serve(
		spanSearchRow("b", "0000000000000002", now.Add(-time.Second)),
		spanSearchRow("c", "0000000000000003", now.Add(-2*time.Second)),
	)
	names, token = findSpans(t, reader, spanSearchQuery(2, token))
	assert.Equal(t, []string{"c"}, names)
	assert.Empty(t, token)

	require.Len(t, conn.RecordedQueries, 2)
	verifyQuerySnapshot(t, conn.RecordedQueries...)
	assert.Equal(t, []any{
		now.Add(-time.Hour).UnixNano(), now.UnixNano(), "cart", uint64(3),
	}, conn.RecordedArgs[0], "the first page fetches one row beyond the page")
	cursorStartNano := now.Add(-time.Second).UnixNano()
	assert.Equal(t, []any{
		now.Add(-time.Hour).UnixNano(), now.UnixNano(), "cart",
		cursorStartNano, cursorStartNano, traceIDHex1, traceIDHex1, "0000000000000002",
		uint64(4),
	}, conn.RecordedArgs[1], "the continuation resumes at the cursor's key and fetches one row more per hash it lists")
}

// TestFindSpans_TimeRangeOnly snapshots the statement of a span query that carries no filter.
func TestFindSpans_TimeRangeOnly(t *testing.T) {
	conn, _ := spanSearchConn(spanSearchRow("a", "0000000000000001", now))
	reader := NewReader(conn, testReaderConfig)
	query := spanSearchQuery(5, "")
	query.Filter = nil
	names, token := findSpans(t, reader, query)
	assert.Equal(t, []string{"a"}, names)
	assert.Empty(t, token)
	require.Len(t, conn.RecordedQueries, 1)
	verifyQuerySnapshot(t, conn.RecordedQueries...)
	assert.Equal(t, []any{now.Add(-time.Hour).UnixNano(), now.UnixNano(), uint64(6)}, conn.RecordedArgs[0])
}

// TestFindSpans_DistinctRowsSharingAKey pages through two rows that tie on the whole sort key
// but differ in content, with ClickHouse returning the tied rows in a different order on each
// request: every row still comes back exactly once.
func TestFindSpans_DistinctRowsSharingAKey(t *testing.T) {
	first := spanSearchRow("a1", "0000000000000001", now)
	second := spanSearchRow("a2", "0000000000000001", now)
	last := spanSearchRow("b", "0000000000000002", now.Add(-time.Second))
	conn, serve := spanSearchConn(first, second, last)
	reader := NewReader(conn, testReaderConfig)

	names, token := findSpans(t, reader, spanSearchQuery(1, ""))
	assert.Equal(t, []string{"a1"}, names)
	require.NotEmpty(t, token)

	serve(second, first, last)
	names, token = findSpans(t, reader, spanSearchQuery(1, token))
	assert.Equal(t, []string{"a2"}, names)
	require.NotEmpty(t, token)

	serve(first, second, last)
	names, token = findSpans(t, reader, spanSearchQuery(1, token))
	assert.Equal(t, []string{"b"}, names)
	assert.Empty(t, token)
}

// TestFindSpans_IdenticalRows pages through a span stored twice: both copies come back.
func TestFindSpans_IdenticalRows(t *testing.T) {
	copy1 := spanSearchRow("a", "0000000000000001", now)
	copy2 := spanSearchRow("a", "0000000000000001", now)
	last := spanSearchRow("b", "0000000000000002", now.Add(-time.Second))
	conn, serve := spanSearchConn(copy1, copy2, last)
	reader := NewReader(conn, testReaderConfig)

	var names []string
	query := spanSearchQuery(1, "")
	for range 3 {
		var page []string
		page, query.Pagination.PageToken = findSpans(t, reader, query)
		names = append(names, page...)
		serve(copy1, copy2, last)
	}
	assert.Equal(t, []string{"a", "a", "b"}, names)
	assert.Empty(t, query.Pagination.PageToken)
}

// TestFindSpans_TiedRunLongerThanTheFetch pages, one span at a time, through four rows that tie
// on the whole key when the server honors the LIMIT and so returns only part of the run on each
// continuation, in an order that changes between requests. The reader stops reading at the page
// plus one row, so a page never grows past that whatever the statement's LIMIT fetched, and
// every row still comes back exactly once.
func TestFindSpans_TiedRunLongerThanTheFetch(t *testing.T) {
	tied := []*dbmodel.SpanRow{
		spanSearchRow("a1", "0000000000000001", now),
		spanSearchRow("a2", "0000000000000001", now),
		spanSearchRow("a3", "0000000000000001", now),
		spanSearchRow("a4", "0000000000000001", now),
	}
	conn, serve := spanSearchConn(tied...)
	reader := NewReader(conn, testReaderConfig)

	var names []string
	query := spanSearchQuery(1, "")
	for page := 0; page < 4; page++ {
		var got []string
		got, query.Pagination.PageToken = findSpans(t, reader, query)
		names = append(names, got...)
		// The next request sees the run rotated by one and cut to three rows, fewer than tie.
		rotated := append(slices.Clone(tied[page+1:]), tied[:page+1]...)
		serve(rotated[:3]...)
	}
	assert.ElementsMatch(t, []string{"a1", "a2", "a3", "a4"}, names)
	assert.Empty(t, query.Pagination.PageToken)
}

// TestFoundSpan_HashIsStable hashes one row many times: a row with a map-valued attribute, which
// the converted span rebuilds through a Go map in a different order on each conversion, hashes
// the same every time because the hash reads the row as stored.
func TestFoundSpan_HashIsStable(t *testing.T) {
	row := spanSearchRow("a", "0000000000000001", now)
	row.Attributes.ComplexKeys = []string{"@map@metadata"}
	row.Attributes.ComplexValues = []string{`{"kvlistValue":{"values":[` +
		`{"key":"k1","value":{"stringValue":"v"}},{"key":"k2","value":{"stringValue":"v"}},` +
		`{"key":"k3","value":{"stringValue":"v"}},{"key":"k4","value":{"stringValue":"v"}},` +
		`{"key":"k5","value":{"stringValue":"v"}},{"key":"k6","value":{"stringValue":"v"}},` +
		`{"key":"k7","value":{"stringValue":"v"}},{"key":"k8","value":{"stringValue":"v"}}]}}`}
	first, err := newFoundSpan(row)
	require.NoError(t, err)
	want := first.hash()
	for range 64 {
		span, err := newFoundSpan(row)
		require.NoError(t, err)
		require.Equal(t, want, span.hash())
	}
}

func TestFindSpans_PageSizeIsClamped(t *testing.T) {
	conn, _ := spanSearchConn()
	reader := NewReader(conn, testReaderConfig)
	query := spanSearchQuery(tracestore.MaxPageSize+1, "")
	names, token := findSpans(t, reader, query)
	assert.Empty(t, names)
	assert.Empty(t, token)
	require.Len(t, conn.RecordedArgs, 1)
	assert.Equal(t, uint64(tracestore.MaxPageSize)+1, conn.RecordedArgs[0][len(conn.RecordedArgs[0])-1])
}

func TestFindSpans_Refusals(t *testing.T) {
	fingerprint, err := spanSearchQuery(1, "").Fingerprint()
	require.NoError(t, err)
	otherQuery := spanSearchQuery(1, "")
	otherQuery.StartTimeMax = now.Add(time.Second)
	otherFingerprint, err := otherQuery.Fingerprint()
	require.NoError(t, err)
	otherToken, err := tracestore.NewPageToken(otherFingerprint, []byte("cursor"))
	require.NoError(t, err)
	shortCursor, err := tracestore.NewPageToken(fingerprint, []byte{1, 2, 3})
	require.NoError(t, err)
	oversized := spanCursor{returned: make([]uint64, maxCursorHashes+1)}
	oversizedCursor, err := tracestore.NewPageToken(fingerprint, oversized.encode())
	require.NoError(t, err)

	tests := []struct {
		name    string
		query   tracestore.SpanQueryParams
		wantErr error
		message string
	}{
		{
			name: "explicit order",
			query: func() tracestore.SpanQueryParams {
				q := spanSearchQuery(1, "")
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
			name:    "token of another query",
			query:   spanSearchQuery(1, otherToken),
			wantErr: tracestore.ErrPaginationInvalid,
			message: "different query",
		},
		{
			name:    "cursor of the wrong shape",
			query:   spanSearchQuery(1, shortCursor),
			wantErr: tracestore.ErrPaginationInvalid,
			message: "does not carry a span position",
		},
		{
			name:    "cursor listing too many spans",
			query:   spanSearchQuery(1, oversizedCursor),
			wantErr: tracestore.ErrPaginationInvalid,
			message: "lists more than",
		},
		{
			name:    "zero page size",
			query:   spanSearchQuery(0, ""),
			wantErr: tracestore.ErrPaginationInvalid,
			message: "page size must be greater than 0",
		},
		{
			name: "unsupported filter",
			query: func() tracestore.SpanQueryParams {
				q := spanSearchQuery(1, "")
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
		err := findSpansError(NewReader(conn, testReaderConfig), spanSearchQuery(1, ""))
		require.ErrorContains(t, err, "failed to query spans: boom")
	})
	t.Run("scan fails", func(t *testing.T) {
		conn, _ := spanSearchConn(spanSearchRow("a", "0000000000000001", now))
		conn.QueryResponses[sql.SelectSpansQuery].Rows.(*clickhousetest.Rows[*dbmodel.SpanRow]).ScanErr = errors.New("boom")
		err := findSpansError(NewReader(conn, testReaderConfig), spanSearchQuery(1, ""))
		require.ErrorContains(t, err, "failed to scan span row: boom")
	})
	t.Run("trace ID does not decode", func(t *testing.T) {
		row := spanSearchRow("a", "0000000000000001", now)
		row.TraceID = "not-hex"
		conn, _ := spanSearchConn(row)
		err := findSpansError(NewReader(conn, testReaderConfig), spanSearchQuery(1, ""))
		require.ErrorContains(t, err, "failed to decode trace ID")
	})
	t.Run("span ID does not decode", func(t *testing.T) {
		row := spanSearchRow("a", "not-hex", now)
		conn, _ := spanSearchConn(row)
		err := findSpansError(NewReader(conn, testReaderConfig), spanSearchQuery(1, ""))
		require.ErrorContains(t, err, "failed to decode span ID")
	})
}

func TestSpanCursor_Encoding(t *testing.T) {
	cursor := spanCursor{
		key: spanKey{
			startTime: now.UnixNano(),
			traceID:   pcommon.TraceID{1, 2, 3},
			spanID:    pcommon.SpanID{4, 5, 6},
		},
		returned: []uint64{7, 8},
	}
	decoded, err := decodeSpanCursor(cursor.encode())
	require.NoError(t, err)
	assert.Equal(t, cursor, decoded)

	for _, size := range []int{0, spanKeySize - 1, spanKeySize + 1} {
		_, err := decodeSpanCursor(make([]byte, size))
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	}
}
