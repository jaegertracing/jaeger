// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// spanSortColumns maps an ordering term's built-in span field to the spans table column that
// carries it. The IDs are fixed-width lower-case hex strings, so their bytewise order is their
// numeric order, and start_time and duration are numeric columns (RFC 0016 §6.3).
var spanSortColumns = map[string]string{
	expression.SpanFieldStartTime: "start_time",
	expression.SpanFieldDuration:  "duration",
	expression.SpanFieldTraceID:   "trace_id",
	expression.SpanFieldSpanID:    "id",
}

// maxCursorHashes bounds the hashes a cursor may list, and with them the rows a continuation
// reads beyond its page. The list holds one hash per row returned so far from the run of rows
// tying on the cursor's key, so it grows by a page per page while the search stays inside one
// run, and only a run of duplicate writes longer than the largest page reaches the bound. The
// reader refuses to mint a cursor past it, with an error that says so, rather than mint a
// token it would refuse on the next request; a received token past it is refused so that a
// forged token cannot size the fetch.
const maxCursorHashes = int(tracestore.MaxPageSize)

// sortClause is one term of a span search's ORDER BY: a spans table column and its direction.
type sortClause struct {
	column     string
	descending bool
}

// placeholder is the bound parameter the clause's column compares against. A start time binds
// as Unix nanoseconds, because clickhouse-go truncates a time.Time argument to seconds.
func (c sortClause) placeholder() string {
	if c.column == "start_time" {
		return "fromUnixTimestamp64Nano(?)"
	}
	return "?"
}

// value is the row's value for the clause, in the form the cursor carries and the placeholder
// binds: an int64 for a numeric column and a string for an ID.
func (c sortClause) value(row *dbmodel.SpanRow) any {
	switch c.column {
	case "start_time":
		return row.StartTime.UnixNano()
	case "duration":
		return row.Duration
	case "trace_id":
		return row.TraceID
	default:
		return row.ID
	}
}

// spanSort lowers the query's order to sort clauses. The effective order ends in the
// tie-breakers, so the clauses name one row up to duplicate writes (RFC 0016 §6.4).
func spanSort(order []tracestore.SpanSortOrder) ([]sortClause, error) {
	order, err := tracestore.EffectiveSpanOrder(order)
	if err != nil {
		return nil, err
	}
	sort := make([]sortClause, 0, len(order))
	for _, term := range order {
		// EffectiveSpanOrder admits only field refs to the fields spanSortColumns maps.
		ref := term.Expression.(*expression.FieldRef)
		sort = append(sort, sortClause{
			column:     spanSortColumns[ref.Name],
			descending: term.Direction == tracestore.SortDescending,
		})
	}
	return sort, nil
}

// spanCursor is where a page ended: the sort values of its last span, one per sort clause, and
// the content hashes of the rows with those values the pages so far have returned. The spans
// table has no column that names one row, and ClickHouse does not promise the same order among
// rows that tie on the whole key from one query to the next, so a count of tied rows would not
// say which of them a page already holds. The hashes do: the continuation re-reads the tied rows
// and drops one occurrence per listed hash, so a span stored twice comes back twice, and two
// rows that tie on the key but differ in content each come back once, whichever order ClickHouse
// returns them in (RFC 0016 §6.4).
type spanCursor struct {
	Key      []any    `json:"key"`
	Returned []uint64 `json:"returned,omitempty"`
}

func (c spanCursor) encode() []byte {
	// The cursor holds only integers and strings, which encode without error.
	b, _ := json.Marshal(c)
	return b
}

// decodeSpanCursor reads the cursor a token carried. A cursor is refused unless it holds one
// value per sort clause, an integer for a numeric column and a string for an ID, since the
// reader never returns one shaped otherwise.
func decodeSpanCursor(raw []byte, sort []sortClause) (spanCursor, error) {
	var wire struct {
		Key      []json.RawMessage `json:"key"`
		Returned []uint64          `json:"returned"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || len(wire.Key) != len(sort) {
		return spanCursor{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	if len(wire.Returned) > maxCursorHashes {
		return spanCursor{}, fmt.Errorf("%w: page token lists more than %d spans at its position", tracestore.ErrPaginationInvalid, maxCursorHashes)
	}
	cursor := spanCursor{Key: make([]any, len(sort)), Returned: wire.Returned}
	for i, clause := range sort {
		var err error
		switch clause.column {
		case "trace_id", "id":
			var s string
			err = json.Unmarshal(wire.Key[i], &s)
			cursor.Key[i] = s
		default:
			var n int64
			err = json.Unmarshal(wire.Key[i], &n)
			cursor.Key[i] = n
		}
		if err != nil {
			return spanCursor{}, fmt.Errorf("%w: page token value %d does not fit the sort column %s", tracestore.ErrPaginationInvalid, i, clause.column)
		}
	}
	return cursor, nil
}

// foundSpan is one row of a span search page: its sort values and the row as scanned.
type foundSpan struct {
	key []any
	row *dbmodel.SpanRow
}

// hash is the content hash a cursor lists for a row at its key. It hashes the row as the spans
// table returns it, whose columns are strings and arrays in stored order, so two reads of one
// row hash alike and so do two rows a duplicate write produced. The converted span would not do:
// a map-valued attribute is rebuilt through a Go map, whose order differs between reads. The
// row is encoded with gob rather than JSON because a double attribute may hold NaN or an
// infinity, which JSON cannot encode and gob writes as the float's bits. Its times are hashed
// in UTC, because clickhouse-go reads a DateTime64 column in the server's time zone, and two
// replicas behind one address may be configured with different zones.
func (s foundSpan) hash() uint64 {
	row := *s.row
	row.StartTime = row.StartTime.UTC()
	row.EventTimestamps = make([]time.Time, len(s.row.EventTimestamps))
	for i, ts := range s.row.EventTimestamps {
		row.EventTimestamps[i] = ts.UTC()
	}
	h := fnv.New64a()
	// The row holds only strings, numbers, times and slices of those, which gob encodes without
	// error, and the hash never fails to write.
	_ = gob.NewEncoder(h).Encode(&row)
	return h.Sum64()
}

func newFoundSpan(row *dbmodel.SpanRow, sort []sortClause) foundSpan {
	key := make([]any, len(sort))
	for i, clause := range sort {
		key[i] = clause.value(row)
	}
	return foundSpan{key: key, row: row}
}

// FindSpans returns one page of the spans matching query, in the query's effective order, with
// the token that resumes the search after them. It selects the span rows directly, with the
// same predicates the trace search assembles and without the trace-ID subquery, ordered by the
// sort clauses and cut at the page size; the cursor inside the token is the last row's sort
// values, and a continuation reads from them onward. See RFC 0016 §6.
func (r *Reader) FindSpans(ctx context.Context, query tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		chunk, err := r.findSpanPage(ctx, query)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		yield(chunk, nil)
	}
}

func (r *Reader) findSpanPage(ctx context.Context, query tracestore.SpanQueryParams) (tracestore.PageChunk[ptrace.Traces], error) {
	var none tracestore.PageChunk[ptrace.Traces]
	sort, err := spanSort(query.OrderBy)
	if err != nil {
		return none, err
	}
	// The query service refuses an explicit order before dispatch, but a reader reached directly
	// must refuse it too while SpanSorting is off.
	capabilities, _ := r.SearchCapabilities(ctx)
	if err := capabilities.ValidateSpanSorting(query.OrderBy); err != nil {
		return none, err
	}
	fingerprint, err := query.Fingerprint()
	if err != nil {
		return none, err
	}
	var after *spanCursor
	if query.Pagination.PageToken != "" {
		raw, err := query.Pagination.PageToken.Cursor(fingerprint)
		if err != nil {
			return none, err
		}
		cursor, err := decodeSpanCursor(raw, sort)
		if err != nil {
			return none, err
		}
		after = &cursor
	}
	// The query service clamps the page size; a caller reaching the reader directly gets the
	// same bound (RFC 0014 §4).
	pageSize := min(query.Pagination.PageSize, tracestore.MaxPageSize)
	if pageSize == 0 {
		return none, fmt.Errorf("%w: page size must be greater than 0", tracestore.ErrPaginationInvalid)
	}
	q, args, err := r.buildFindSpansQuery(ctx, query, sort, after, pageSize)
	if err != nil {
		return none, fmt.Errorf("failed to build query: %w", err)
	}
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return none, fmt.Errorf("failed to query spans: %w", err)
	}
	// One row beyond the page tells whether another page exists.
	found, err := readFoundSpans(rows, sort, after, pageSize+1)
	if err != nil {
		return none, err
	}
	more := uint64(len(found)) > uint64(pageSize)
	if more {
		found = found[:pageSize]
	}
	chunk := tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()}
	if more {
		next := nextSpanCursor(found, after)
		if len(next.Returned) > maxCursorHashes {
			return none, fmt.Errorf("span search cannot page past %d spans that tie on the sort key", maxCursorHashes)
		}
		if chunk.NextPageToken, err = tracestore.NewPageToken(fingerprint, next.encode()); err != nil {
			return none, err
		}
	}
	// Every span keeps its own ResourceSpans, so the page keeps the SQL order even where
	// consecutive spans alternate between services.
	for _, span := range found {
		dbmodel.FromRow(span.row).ResourceSpans().MoveAndAppendTo(chunk.Results.ResourceSpans())
	}
	return chunk, nil
}

// buildFindSpansQuery selects the span rows the query matches, from the cursor's key onward
// when there is one, ordered by the sort clauses. It fetches one row beyond the page, which
// tells whether another page exists, and one more per hash the cursor lists, since those rows
// are read again and dropped.
func (r *Reader) buildFindSpansQuery(
	ctx context.Context,
	query tracestore.SpanQueryParams,
	sort []sortClause,
	after *spanCursor,
	pageSize uint32,
) (string, []any, error) {
	var q strings.Builder
	q.WriteString(strings.TrimRight(sql.SelectSpansQuery, "\n"))
	q.WriteString("\nWHERE 1=1")
	var args []any
	// Positional time.Time arguments are truncated to seconds by clickhouse-go.
	if !query.StartTimeMin.IsZero() {
		appendAnd(&q, "s.start_time >= fromUnixTimestamp64Nano(?)")
		args = append(args, query.StartTimeMin.UnixNano())
	}
	if !query.StartTimeMax.IsZero() {
		appendAnd(&q, "s.start_time <= fromUnixTimestamp64Nano(?)")
		args = append(args, query.StartTimeMax.UnixNano())
	}
	if query.Filter != nil {
		var err error
		if args, err = r.appendFilter(ctx, &q, args, query.Filter); err != nil {
			return "", nil, err
		}
	}
	fetch := uint64(pageSize) + 1
	if after != nil {
		appendAnd(&q, "")
		args = appendContinuation(&q, args, sort, after.Key)
		fetch += uint64(len(after.Returned))
	}
	q.WriteString("\nORDER BY ")
	for i, clause := range sort {
		if i > 0 {
			q.WriteString(", ")
		}
		q.WriteString("s." + clause.column)
		if clause.descending {
			q.WriteString(" DESC")
		} else {
			q.WriteString(" ASC")
		}
	}
	q.WriteString("\nLIMIT ?")
	args = append(args, fetch)
	return q.String(), args, nil
}

// appendContinuation writes the predicate that selects the rows at or after key in the order
// of the sort clauses: the first clause is past its value, or equal to it and the remaining
// clauses are at or after theirs. The last clause compares inclusively, so that the rows tying
// on the whole key are read again and the cursor's hashes decide which of them the earlier
// pages hold.
func appendContinuation(q *strings.Builder, args []any, sort []sortClause, key []any) []any {
	clause := sort[0]
	past := ">"
	if clause.descending {
		past = "<"
	}
	column := "s." + clause.column
	if len(sort) == 1 {
		q.WriteString(column + " " + past + "= " + clause.placeholder())
		return append(args, key[0])
	}
	q.WriteString("(" + column + " " + past + " " + clause.placeholder() + " OR (" + column + " = " + clause.placeholder() + " AND ")
	args = append(args, key[0], key[0])
	args = appendContinuation(q, args, sort[1:], key[1:])
	q.WriteString("))")
	return args
}

// readFoundSpans scans rows into foundSpans until it holds want of them, dropping the rows at
// the cursor's key that the cursor says earlier pages returned, one row per listed hash. It
// stops reading at want rather than trust the statement's LIMIT, which a token sized.
func readFoundSpans(rows driver.Rows, sort []sortClause, after *spanCursor, want uint32) ([]foundSpan, error) {
	var returned []uint64
	if after != nil {
		returned = slices.Clone(after.Returned)
	}
	var found []foundSpan
	err := readRows(rows, "span", func(rows driver.Rows) error {
		row, err := dbmodel.ScanRow(rows)
		if err != nil {
			return fmt.Errorf("failed to scan span row: %w", err)
		}
		span := newFoundSpan(row, sort)
		if after != nil && slices.Equal(span.key, after.Key) {
			if i := slices.Index(returned, span.hash()); i >= 0 {
				returned = slices.Delete(returned, i, i+1)
				return nil
			}
		}
		found = append(found, span)
		if uint64(len(found)) == uint64(want) {
			return errStopReading
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopReading) {
		return nil, err
	}
	return found, nil
}

// nextSpanCursor is the cursor after a full page: the sort values of its last span, and the
// hashes of every span in the page with those values, plus those the incoming cursor listed
// when the page never left its key.
func nextSpanCursor(page []foundSpan, after *spanCursor) spanCursor {
	next := spanCursor{Key: page[len(page)-1].key}
	for i := len(page) - 1; i >= 0 && slices.Equal(page[i].key, next.Key); i-- {
		next.Returned = append(next.Returned, page[i].hash())
	}
	if after != nil && slices.Equal(after.Key, next.Key) {
		next.Returned = append(next.Returned, after.Returned...)
	}
	return next
}
