// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

type searchTestConn struct {
	driver.Conn
	query func(context.Context, string, ...any) (driver.Rows, error)
}

func (c *searchTestConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	return c.query(ctx, query, args...)
}

type searchTestRows struct {
	driver.Rows
	closed bool
}

func (r *searchTestRows) Close() error {
	r.closed = true
	return r.Rows.Close()
}

func TestFindTraces_StagedQueries(t *testing.T) {
	start := time.Unix(1700000000, 123456789)
	end := start.Add(3*time.Second + 987654321*time.Nanosecond)
	candidateRows := &searchTestRows{Rows: &clickhousetest.Rows[[]any]{
		Data: [][]any{{traceIDHex1}, {traceIDHex2}}, ScanFn: scanTraceIDFn(),
	}}
	timestampRows := &searchTestRows{Rows: &clickhousetest.Rows[[]any]{
		// Metadata order need not match the candidate order.
		Data:   [][]any{{traceIDHex2, start.Add(time.Second), end}, {traceIDHex1, start, end.Add(-time.Second)}},
		ScanFn: scanTraceIDFn(),
	}}
	spanRows := &searchTestRows{Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{Data: multipleSpans, ScanFn: scanSpanRowFn()}}
	query := tracestore.TraceQueryParams{
		ServiceName: "serviceA", Attributes: pcommon.NewMap(), SearchDepth: 2,
		StartTimeMin: start.Add(time.Second), StartTimeMax: end.Add(-time.Second),
	}
	calls := 0
	conn := &searchTestConn{query: func(ctx context.Context, q string, args ...any) (driver.Rows, error) {
		require.Equal(t, t.Context(), ctx)
		calls++
		switch calls {
		case 1:
			require.Contains(t, q, sql.SearchTraceIDsBase)
			require.NotContains(t, q, "JOIN")
			require.NotContains(t, q, "trace_id_timestamps")
			require.Equal(t, []any{"serviceA", query.StartTimeMin, query.StartTimeMax, uint32(2)}, args)
			return candidateRows, nil
		case 2:
			require.True(t, candidateRows.closed)
			require.Equal(t, sql.SelectTraceIDTimestamps, q)
			require.Contains(t, q, "WHERE trace_id IN (?)")
			require.Contains(t, q, "min(start) AS start")
			require.Contains(t, q, "max(end) AS end")
			require.Contains(t, q, "GROUP BY trace_id")
			require.Equal(t, []any{[]string{traceIDHex1, traceIDHex2}}, args)
			return timestampRows, nil
		case 3:
			require.True(t, timestampRows.closed)
			require.True(t, strings.HasPrefix(q, strings.TrimRight(sql.SelectSpansQuery, "\n")))
			require.Contains(t, q, "WHERE (s.trace_id = ? AND s.start_time >= fromUnixTimestamp64Nano(?) AND s.start_time <= fromUnixTimestamp64Nano(?)) OR (s.trace_id = ? AND s.start_time >= fromUnixTimestamp64Nano(?) AND s.start_time <= fromUnixTimestamp64Nano(?))")
			require.NotContains(t, q, "s.service_name =")
			require.NotContains(t, q, "trace_id_timestamps")
			require.Equal(t, []any{
				traceIDHex1, start.UnixNano(), end.Add(-time.Second).UnixNano(),
				traceIDHex2, start.Add(time.Second).UnixNano(), end.UnixNano(),
			}, args)
			return spanRows, nil
		default:
			t.Fatalf("unexpected query: %s", q)
			return nil, nil
		}
	}}
	reader := NewReader(conn, testReaderConfig)
	traces, err := jiter.FlattenWithErrors(reader.FindTraces(t.Context(), query))
	require.NoError(t, err)
	requireTracesEqual(t, multipleSpans, traces)
	require.Equal(t, 3, calls)
	require.True(t, spanRows.closed)
}

func TestFindTraces_NoCandidates(t *testing.T) {
	for _, method := range []string{"FindTraces", "FindTraceIDs"} {
		t.Run(method, func(t *testing.T) {
			rows := &searchTestRows{Rows: &clickhousetest.Rows[[]any]{}}
			calls := 0
			conn := &searchTestConn{query: func(_ context.Context, q string, _ ...any) (driver.Rows, error) {
				calls++
				require.Contains(t, q, sql.SearchTraceIDsBase)
				return rows, nil
			}}
			reader := NewReader(conn, testReaderConfig)
			query := tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}
			if method == "FindTraces" {
				traces, err := jiter.FlattenWithErrors(reader.FindTraces(t.Context(), query))
				require.NoError(t, err)
				require.Empty(t, traces)
			} else {
				ids, err := flattenPageChunks(reader.FindTraceIDs(t.Context(), query))
				require.NoError(t, err)
				require.Empty(t, ids)
			}
			require.Equal(t, 1, calls)
			require.True(t, rows.closed)
		})
	}
}

func TestFindTraceIDs_MissingTimestamps(t *testing.T) {
	candidates := &clickhousetest.Rows[[]any]{Data: [][]any{{traceIDHex1}, {traceIDHex2}}, ScanFn: scanTraceIDFn()}
	timestamps := &clickhousetest.Rows[[]any]{
		Data:   [][]any{{traceIDHex2, now.Add(-time.Hour), now}, {"00000000000000000000000000000003", now, now}},
		ScanFn: scanTraceIDFn(),
	}
	conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
		sql.SearchTraceIDsBase: {Rows: candidates}, sql.SelectTraceIDTimestamps: {Rows: timestamps},
	}}
	reader := NewReader(conn, testReaderConfig)
	ids, err := flattenPageChunks(reader.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}))
	require.NoError(t, err)
	require.Equal(t, []tracestore.FoundTraceID{
		{TraceID: pcommon.TraceID{15: 1}},
		{TraceID: pcommon.TraceID{15: 2}, Start: now.Add(-time.Hour), End: now},
	}, ids)
	q, args := buildFindTracesQuery(ids)
	require.Equal(t, strings.TrimRight(sql.SelectSpansQuery, "\n")+
		"\nWHERE (s.trace_id = ?) OR (s.trace_id = ? AND s.start_time >= fromUnixTimestamp64Nano(?) AND s.start_time <= fromUnixTimestamp64Nano(?))\nORDER BY s.trace_id", q)
	require.Equal(t, []any{traceIDHex1, traceIDHex2, now.Add(-time.Hour).UnixNano(), now.UnixNano()}, args)
}

func TestBuildFindTracesQuery_TimeBounds(t *testing.T) {
	start := time.Unix(1700000000, 123456789)
	end := start.Add(3*time.Second + 987654321*time.Nanosecond)
	const boundedTrace = "(s.trace_id = ? AND s.start_time >= fromUnixTimestamp64Nano(?) AND s.start_time <= fromUnixTimestamp64Nano(?))"
	for _, tc := range []struct {
		name      string
		bounds    []tracestore.FoundTraceID
		condition string
		args      []any
	}{
		{
			name: "single trace", bounds: []tracestore.FoundTraceID{{TraceID: pcommon.TraceID{15: 1}, Start: start, End: end}},
			condition: boundedTrace,
			args:      []any{traceIDHex1, start.UnixNano(), end.UnixNano()},
		},
		{
			name: "long trace does not widen another trace's range", bounds: []tracestore.FoundTraceID{
				{TraceID: pcommon.TraceID{15: 1}, Start: start.Add(time.Second), End: end.Add(-time.Second)},
				{TraceID: pcommon.TraceID{15: 2}, Start: start.Add(-24 * time.Hour), End: end.Add(24 * time.Hour)},
			},
			condition: boundedTrace + " OR " + boundedTrace,
			args: []any{
				traceIDHex1, start.Add(time.Second).UnixNano(), end.Add(-time.Second).UnixNano(),
				traceIDHex2, start.Add(-24 * time.Hour).UnixNano(), end.Add(24 * time.Hour).UnixNano(),
			},
		},
		{
			name: "mixed metadata", bounds: []tracestore.FoundTraceID{
				{TraceID: pcommon.TraceID{15: 1}, Start: start, End: end},
				{TraceID: pcommon.TraceID{15: 2}},
				{TraceID: pcommon.TraceID{15: 3}, Start: start.Add(-time.Hour), End: end.Add(time.Hour)},
			},
			condition: boundedTrace + " OR (s.trace_id = ?) OR " + boundedTrace,
			args: []any{
				traceIDHex1, start.UnixNano(), end.UnixNano(), traceIDHex2,
				"00000000000000000000000000000003", start.Add(-time.Hour).UnixNano(), end.Add(time.Hour).UnixNano(),
			},
		},
		{
			name: "missing start", bounds: []tracestore.FoundTraceID{{TraceID: pcommon.TraceID{15: 1}, End: end}},
			condition: "(s.trace_id = ?)",
			args:      []any{traceIDHex1},
		},
		{
			name: "missing end", bounds: []tracestore.FoundTraceID{{TraceID: pcommon.TraceID{15: 1}, Start: start}},
			condition: "(s.trace_id = ?)",
			args:      []any{traceIDHex1},
		},
		{
			name: "all metadata missing", bounds: []tracestore.FoundTraceID{
				{TraceID: pcommon.TraceID{15: 1}}, {TraceID: pcommon.TraceID{15: 2}},
			},
			condition: "(s.trace_id = ?) OR (s.trace_id = ?)",
			args:      []any{traceIDHex1, traceIDHex2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, args := buildFindTracesQuery(tc.bounds)
			require.Equal(t, strings.TrimRight(sql.SelectSpansQuery, "\n")+"\nWHERE "+tc.condition+"\nORDER BY s.trace_id", q)
			require.Equal(t, tc.args, args)
		})
	}
}

func TestFindTraceIDs_StageErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stage    int
		rows     *clickhousetest.Rows[[]any]
		queryErr error
		expected string
	}{
		{name: "candidate close", stage: 1, rows: &clickhousetest.Rows[[]any]{CloseErr: assert.AnError}, expected: "failed to close rows"},
		{name: "timestamp query", stage: 2, queryErr: assert.AnError, expected: "failed to query trace timestamps"},
		{name: "timestamp scan", stage: 2, rows: &clickhousetest.Rows[[]any]{Data: testTraceIDsData, ScanErr: assert.AnError}, expected: "failed to scan row"},
		{name: "timestamp decode", stage: 2, rows: &clickhousetest.Rows[[]any]{Data: [][]any{{"invalid", now, now}}, ScanFn: scanTraceIDFn()}, expected: "failed to decode trace ID"},
		{name: "timestamp iteration", stage: 2, rows: &clickhousetest.Rows[[]any]{RowsErr: assert.AnError}, expected: "failed to read trace timestamp rows"},
		{name: "timestamp close", stage: 2, rows: &clickhousetest.Rows[[]any]{CloseErr: assert.AnError}, expected: "failed to close rows"},
		{name: "timestamp canceled", stage: 2, queryErr: context.Canceled, expected: "failed to query trace timestamps"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var checkedRows *searchTestRows
			candidateRows := &searchTestRows{Rows: &clickhousetest.Rows[[]any]{Data: testTraceIDsData, ScanFn: scanTraceIDFn()}}
			conn := &searchTestConn{query: func(ctx context.Context, _ string, _ ...any) (driver.Rows, error) {
				require.Equal(t, t.Context(), ctx)
				calls++
				if calls == tc.stage {
					if tc.rows != nil {
						checkedRows = &searchTestRows{Rows: tc.rows}
						return checkedRows, nil
					}
					return nil, tc.queryErr
				}
				require.Equal(t, 1, calls)
				return candidateRows, nil
			}}
			reader := NewReader(conn, testReaderConfig)
			_, err := flattenPageChunks(reader.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}))
			require.ErrorContains(t, err, tc.expected)
			if tc.queryErr != nil {
				require.ErrorIs(t, err, tc.queryErr)
			}
			require.Equal(t, tc.stage, calls)
			if checkedRows != nil {
				require.True(t, checkedRows.closed)
			}
			if tc.stage == 2 {
				require.True(t, candidateRows.closed)
			}
		})
	}
}

func TestFindTraces_CloseError(t *testing.T) {
	conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
		sql.SelectSpansQuery: {Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{CloseErr: assert.AnError}},
	}}
	addTraceSearchResponses(conn)
	reader := NewReader(conn, testReaderConfig)
	_, err := jiter.FlattenWithErrors(reader.FindTraces(t.Context(), tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}))
	require.ErrorContains(t, err, "failed to close rows")
	require.ErrorIs(t, err, assert.AnError)
}
